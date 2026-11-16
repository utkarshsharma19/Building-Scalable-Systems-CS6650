terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.7.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.0"
    }
  }
}

# Personal AWS account (profile "hw8"), not Learner Lab: we create our own least-privilege IAM roles.
provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile
  default_tags {
    tags = { Project = "cs6650-hw8", Part = "step2-dynamodb" }
  }
}

data "aws_ecr_authorization_token" "registry" {}

provider "docker" {
  host = "unix://${pathexpand("~/.docker/run/docker.sock")}"
  registry_auth {
    address  = data.aws_ecr_authorization_token.registry.proxy_endpoint
    username = data.aws_ecr_authorization_token.registry.user_name
    password = data.aws_ecr_authorization_token.registry.password
  }
}
