provider "aws" {
  region = "eu-west-1"
}

# terraform {
#   backend "s3" {
#     bucket = "mybucket"
#     key    = "path/to/my/key"
#     region = "us-east-1"
#   }
# }

data "aws_ami" "amazon_linux" {
  most_recent = true

  owners = ["amazon"]

  filter {
    name = "name"

    values = [
      "amzn-ami-hvm-*-x86_64-gp2",
    ]
  }

  filter {
    name = "owner-alias"

    values = [
      "amazon",
    ]
  }
}

data "aws_vpcs" "selected" {
  tags = {
    service = "secure"
  }
}

locals {
  vpc_ids = coalesce(tolist(var.vpc_ids), data.aws_vpcs.selected.ids)
  subnet_ids = coalesce(tolist(var.subnet_ids), data.aws_subnet_ids.secure.ids)
}

data "aws_security_groups" "secure" {
  filter {
    name   = "group-name"
    values = ["private"]
  }
  filter {
    name   = "vpc-id"
    values = local.vpc_ids
  }
}

data "aws_subnet_ids" "secure" {
  vpc_id = local.vpc_ids[0]

  tags = {
    Tier = "Private"
  }
}


module "ec2-instance" {
  source  = "terraform-aws-modules/ec2-instance/aws"
  version = "2.15.0"

  ami                         = data.aws_ami.amazon_linux.id
  associate_public_ip_address = var.associate_public_ip_address
  instance_type               = var.instance_type

  name                   = var.instance_name
  private_ip             = var.private_ip
  subnet_id              = local.subnet_ids[0]
  vpc_security_group_ids = data.aws_security_groups.secure.ids
  ebs_block_device = var.ebs_block_device
}

output "private_ip" {
  value = module.ec2-instance.private_ip
}