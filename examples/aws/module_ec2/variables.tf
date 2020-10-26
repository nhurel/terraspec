variable "associate_public_ip_address" {
  default = false
}

variable "instance_type" {
  default = "t2.small"
}
variable "instance_name" {
  default = "my-instance"
}

variable "private_ip" {
  default = ""
}

variable "vpc_ids"{
  description = "Override vpc_ids instead of using datasource. Useful for tests"
  default= []
}
variable "subnet_ids"{
  description = "Override subnet_ids instead of using datasource. Useful for tests"
  default= []
}

variable "ebs_block_device"{
  type = list(map(string))
  default = []
}