  num                               = 10
  cluster_name                      = "test"
  dcos_instance_os                  = "centos_7.6"
  tags                              = {"cluster-type":"mono"}
  hostname_format                   = "%[2]s-instance%02.[1]f"
  name_prefix                       = "mono"
  avset_platform_fault_domain_count = "1"