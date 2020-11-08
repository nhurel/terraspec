variable "num" {
  description = "How many instances should be created"
}

variable "public_ssh_key"{
    default = "/dev/null"
}

variable "location" {
  description = "Azure Region"
  default     = "uksouth"
}

variable "cluster_name" {
  description = "Name of the DC/OS cluster"
}

variable "vm_size" {
  description = "Azure virtual machine size"
  default     = "Standard_DS1_v2"
}

variable "dcos_instance_os" {
  description = "Operating system to use. Instead of using your own AMI you could use a provided OS."
}

variable "disk_size" {
  description = "Disk Size in GB"
  default     = "100"
}

variable "resource_group_name" {
  description = "Name of the azure resource group"
  default     = "test"
}

variable "tags" {
  description = "Add custom tags to all resources"
  type        = map
  default     = {}
}

variable "hostname_format" {
  description = "Format the hostname inputs are index+1, region, cluster_name"
  default     = "instance-%[1]d-%[2]s"
}


variable "subnet_id" {
  description = "Subnet ID"
  default     = ""
}

variable "name_prefix" {
  description = "Name Prefix"
}

variable "avset_platform_fault_domain_count" {
  description = "Availability set platform fault domain count, differs from location to location"
  default     = 3
}