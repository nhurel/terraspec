module "instance" {
  source  = "dcos-terraform/instance/azurerm"
  version = "0.3.0"

  num                               = var.num
  location                          = var.location
  cluster_name                      = var.cluster_name
  vm_size                           = var.vm_size
  dcos_instance_os                  = var.dcos_instance_os
  disk_size                         = var.disk_size
  resource_group_name               = var.resource_group_name
  tags                              = var.tags
  hostname_format                   = var.hostname_format
  subnet_id                         = var.subnet_id
  name_prefix                       = var.name_prefix
  avset_platform_fault_domain_count = var.avset_platform_fault_domain_count
  public_ssh_key = var.public_ssh_key
}
