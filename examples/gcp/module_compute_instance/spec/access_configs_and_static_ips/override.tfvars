
hostname = "myinstance"
static_ips = ["10.0.0.10","10.0.0.11","10.0.0.12"]
access_config = [
    {nat_ip: "10.10.0.20", network_tier:"dmz"},
    {nat_ip: "10.10.0.21", network_tier:"dmz"},
    {nat_ip: "10.10.0.22", network_tier:"dmz"}
    ]
num_instances = "1"
