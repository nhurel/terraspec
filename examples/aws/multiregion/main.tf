terraform {
  required_version = "~>0.14.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 2.0"
    }
  }
}

provider "aws" {
  region = "eu-west-1"
}

provider "aws" {
  alias = "eu-west-2"
  region = "eu-west-2"
}

data "aws_region" "current" {}
data "aws_region" "west2" {
    provider = aws.eu-west-2
}


output "eu-west-1" {
  value = data.aws_region.current.name
}
output "eu-west-2" {
  value = data.aws_region.west2.name
}