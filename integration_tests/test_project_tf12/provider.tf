provider "cloudfoundry" {
  version = "0.12.4"
}

provider "aws" {
  region = "us"
  version = "3.8.0"
}

data "aws_vpc" "vpc" {
  id = "vpc-id"
}


data "cloudfoundry_org" "org" {
  name = "my-org"
}

resource "cloudfoundry_space" "space" {
    name = "my-space"
    org = data.cloudfoundry_org.org.id
}
