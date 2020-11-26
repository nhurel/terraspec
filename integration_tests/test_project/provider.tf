terraform {
  required_providers {
    cloudfoundry = {
      source  = "no.registry.com/nocorp/cloudfoundry"
      version = "0.12.4"
    }
    aws = {
      source = "hashicorp/aws"
      version = "3.8.0"
    }
  }
}

provider "cloudfoundry" {
}

provider "aws" {
  region = "us"
}

data "aws_vpc" "vpc" {
  id = "vpc-id"
}


data "aws_region" "current" {}
data "aws_iam_account_alias" "current" {}
data "aws_billing_service_account" "main" {}


data "cloudfoundry_org" "org" {
  name = "my-org"
}

resource "cloudfoundry_space" "space" {
    name = "my-space"
    org = data.cloudfoundry_org.org.id
}

output "region_name"{
  value = data.aws_region.current.name
}
output "account_id"{
  value = data.aws_iam_account_alias.current.id
}

output "billing_account_id"{
  value = data.aws_billing_service_account.main.id
}