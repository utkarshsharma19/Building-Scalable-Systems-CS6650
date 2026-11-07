terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.7.0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 2.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

data "aws_ecr_authorization_token" "registry" {}

provider "docker" {
  # Docker Desktop on macOS listens here, not on /var/run/docker.sock.
  host = "unix://${pathexpand("~/.docker/run/docker.sock")}"

  registry_auth {
    address  = data.aws_ecr_authorization_token.registry.proxy_endpoint
    username = data.aws_ecr_authorization_token.registry.user_name
    password = data.aws_ecr_authorization_token.registry.password
  }
}

# Learner Lab: every service uses the pre-made LabRole.
data "aws_iam_role" "lab_role" {
  name = "LabRole"
}
