provider "aws" {
  region = "eu-west-1"
}

data "terraform_remote_state" "vpc" {
   backend = "s3"
   config = {
     bucket = "mybucket"
     key    = "path/to/my/key"
     region = "us-east-1"
   }
 }

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

data "aws_subnet_ids" "secure" {
  vpc_id = element(tolist(data.aws_vpcs.selected.ids), 0)

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
  subnet_id              = element(tolist(data.aws_subnet_ids.secure.ids), 0)
  vpc_security_group_ids = data.terraform_remote_state.vpc.outputs.security_groups
  ebs_block_device = var.ebs_block_device
}

output "private_ip" {
  value = module.ec2-instance.private_ip
}

output "arn" {
  value = module.ec2-instance.arn
}
