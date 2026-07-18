# pip install dnspython

import asyncio
import random
import time
import dns.message
import dns.rdatatype
import dns.asyncquery


# Global variables
NAMESERVER_IP = "127.0.0.1"  # Set to your own project's DNS server IP
NAMESERVER_PORT = 5553

IS_LOAD_TEST = True
MAX_TEST_COUNT = 5

# List of 20 domain names
DOMAINS = [
    "google.com", "apple.com", "microsoft.com", "amazon.com", "cloudflare.com",
    "github.com", "netflix.com", "yahoo.com", "wikipedia.org", "meta.com",
    "reddit.com", "x.com", "linkedin.com", "taobao.com", "qq.com",
    "jd.com", "openai.com", "baidu.com", "csdn.net", "ibm.com"
]

# Supported DNS record types
RECORD_TYPES = ["A", "AAAA", "TYPE65"]

async def send_dns_query(domain, q_type, show_log=False):
    """
    Send a single async UDP DNS query.
    """
    try:
        # Convert record type string to dnspython object
        if q_type.upper() == "TYPE65":
            rtype = dns.rdatatype.HTTPS
        else:
            rtype = dns.rdatatype.from_text(q_type)
            
        # Build the DNS query
        query_packet = dns.message.make_query(domain, rtype)
        
        # Send async UDP request
        response = await dns.asyncquery.udp(
            query_packet, 
            NAMESERVER_IP, 
            port=NAMESERVER_PORT, 
            timeout=2.0
        )
        
        if show_log:
            print(f"[{domain} - {q_type}] Success! Found {len(response.answer)} records.")
            
    except Exception as error:
        if show_log:
            print(f"[{domain} - {q_type}] Failed: {error}")

async def run_normal_mode():
    """
    Request randomly from domains every 10 seconds.
    """
    print(f"Running normal mode: 1 request every 10 seconds to {NAMESERVER_IP}:{NAMESERVER_PORT}")
    count = 0
    while count < MAX_TEST_COUNT: 
        count += 1  # Run for MAX_TEST_COUNT iterations
        target_domain = random.choice(DOMAINS)
        target_type = random.choice(RECORD_TYPES)
        
        await send_dns_query(target_domain, target_type, show_log=True)
        await asyncio.sleep(10)

async def run_load_test_mode():
    """
    Attempt to send 10,000 requests per second for testing.
    """
    print(f"Running load test mode: Target 10,000 QPS to {NAMESERVER_IP}:{NAMESERVER_PORT}")
    target_qps = 10000
    count = 0

    while count < MAX_TEST_COUNT:  # Run for MAX_TEST_COUNT iterations
        count += 1
        start_time = time.time()
        
        # Create a batch of tasks
        tasks = []
        for _ in range(target_qps):
            target_domain = random.choice(DOMAINS)
            target_type = random.choice(RECORD_TYPES)
            # Disable logging in load test to save CPU and IO
            tasks.append(send_dns_query(target_domain, target_type, show_log=False))
            
        # Run all tasks concurrently
        await asyncio.gather(*tasks, return_exceptions=True)
        
        elapsed_time = time.time() - start_time
        print(f"Dispatched {target_qps} queries in {elapsed_time:.4f} seconds.")
        
        # If the batch finished in less than 1 second, sleep to maintain QPS rate
        if elapsed_time < 1.0:
            await asyncio.sleep(1.0 - elapsed_time)

async def main():
    if IS_LOAD_TEST:
        await run_load_test_mode()
    else:
        await run_normal_mode()

if __name__ == "__main__":
    # Use standard asyncio event loop
    asyncio.run(main())