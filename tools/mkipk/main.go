// mkipk assembles an OpenWrt .ipk package from a cross-compiled dnsforvpn
// binary. Pure stdlib: the .ipk is an ar archive containing
// debian-binary, control.tar.gz and data.tar.gz — written without any
// binutils/ar dependency so it works on any build host (including Windows).
//
// Usage:
//
//	go run ./tools/mkipk -binary <path> -arch <ipk-arch> -version <ver> -out <file.ipk>
//
// Example:
//
//	go run ./tools/mkipk -binary dnsforvpn -arch aarch64_generic -version 0.2.0 -out dnsforvpn.ipk
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// fileToPack is one member of a tar archive.
type fileToPack struct {
	name string
	mode int64
	data []byte
}

func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read", path+":", err)
		os.Exit(1)
	}
	return b
}

// tarGz packs files into a gzipped tar in memory.
func tarGz(files []fileToPack) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, f := range files {
		typeflag := byte(tar.TypeReg)
		if f.data == nil {
			typeflag = tar.TypeDir
		}
		hdr := &tar.Header{
			Name:     f.name,
			Mode:     f.mode,
			Size:     int64(len(f.data)),
			ModTime:  time.Now(),
			Typeflag: typeflag,
			Uid:      0,
			Gid:      0,
			Uname:    "root",
			Gname:    "root",
		}
		if err := tw.WriteHeader(hdr); err != nil {
			fatal(err)
		}
		if _, err := tw.Write(f.data); err != nil {
			fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		fatal(err)
	}
	if err := gw.Close(); err != nil {
		fatal(err)
	}
	return buf.Bytes()
}

// arWrite appends one member to an ar (GNU variant) archive.
func arWrite(buf *bytes.Buffer, name string, data []byte, mode int64) {
	// GNU ar terminates file names with '/'.
	hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
		name+"/", time.Now().Unix(), 0, 0, mode, len(data))
	if len(hdr) != 60 {
		fatal(fmt.Errorf("ar header for %s is %d bytes, want 60", name, len(hdr)))
	}
	buf.WriteString(hdr)
	buf.Write(data)
	if len(data)%2 == 1 {
		buf.WriteByte('\n') // members are 2-byte aligned
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mkipk:", err)
	os.Exit(1)
}

func main() {
	binaryPath := flag.String("binary", "", "path to the cross-compiled dnsforvpn binary")
	arch := flag.String("arch", "", "opkg architecture, e.g. aarch64_generic, mipsel_24kc, x86_64")
	version := flag.String("version", "0.2.0", "package version")
	out := flag.String("out", "", "output .ipk path")
	flag.Parse()
	if *binaryPath == "" || *arch == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: mkipk -binary <path> -arch <ipk-arch> [-version v] -out <file.ipk>")
		os.Exit(2)
	}

	binary := mustRead(*binaryPath)
	control := mustRead("deploy/openwrt/control")
	control = []byte(strings.NewReplacer("@VERSION@", *version, "@ARCH@", *arch).Replace(string(control)))

	dataTar := tarGz([]fileToPack{
		{"./usr/", 0755, nil},
		{"./usr/bin/", 0755, nil},
		{"./usr/bin/dnsforvpn", 0755, binary},
		{"./etc/", 0755, nil},
		{"./etc/init.d/", 0755, nil},
		{"./etc/init.d/dnsforvpn", 0755, mustRead("deploy/openwrt/dnsforvpn.init")},
		{"./etc/dnsforvpn/", 0755, nil},
		{"./etc/dnsforvpn/config.toml", 0644, mustRead("deploy/openwrt/config.toml")},
		{"./etc/dnsforvpn/rules/", 0755, nil},
		// 种子规则文件随包携带: 首次启动无需访问 raw.githubusercontent.com
		{"./etc/dnsforvpn/rules/gfwlist.txt", 0644, mustRead("configs/rules/gfwlist.txt")},
	})

	controlTar := tarGz([]fileToPack{
		{"./control", 0644, control},
		{"./postinst", 0755, mustRead("deploy/openwrt/postinst")},
		{"./prerm", 0755, mustRead("deploy/openwrt/prerm")},
	})

	var pkg bytes.Buffer
	pkg.WriteString("!<arch>\n")
	arWrite(&pkg, "debian-binary", []byte("2.0\n"), 0644)
	arWrite(&pkg, "control.tar.gz", controlTar, 0644)
	arWrite(&pkg, "data.tar.gz", dataTar, 0644)

	if err := os.WriteFile(*out, pkg.Bytes(), 0644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", *out, pkg.Len())
}
