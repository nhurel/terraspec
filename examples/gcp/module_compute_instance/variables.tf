variable "network" {
  default = "my-network"
}

variable "subnetwork_project" {
  description = "The project that subnetwork belongs to"
  default     = "my-project"
}

variable "hostname" {
  default = ""
}

variable "static_ips" {
  type        = list(string)
  description = "List of static IPs for VM instances"
  default     = []
}

variable "access_config" {
  description = "Access configurations, i.e. IPs via which the VM instance can be accessed via the Internet."
  type = list(object({
    nat_ip       = string
    network_tier = string
  }))
  default = []
}

variable "num_instances" {
  description = "Number of instances to create. This value is ignored if static_ips is provided."
  default     = "1"
}

variable "instance_template" {
  default = "my-template"
}

variable "region" {
  default = "eu-west1"
}

