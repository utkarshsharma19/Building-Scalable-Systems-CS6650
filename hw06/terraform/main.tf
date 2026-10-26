module "network" {
  source         = "./modules/network"
  service_name   = var.service_name
  container_port = var.container_port
}

module "ecr" {
  source          = "./modules/ecr"
  repository_name = var.ecr_repository_name
}

module "logging" {
  source            = "./modules/logging"
  service_name      = var.service_name
  retention_in_days = var.log_retention_days
}

data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

module "alb" {
  source               = "./modules/alb"
  count                = var.horizontal_scaling ? 1 : 0
  service_name         = var.service_name
  vpc_id               = module.network.vpc_id
  subnet_ids           = module.network.subnet_ids
  container_port       = var.container_port
  deregistration_delay = var.deregistration_delay
}

module "ecs" {
  source             = "./modules/ecs"
  service_name       = var.service_name
  image              = "${module.ecr.repository_url}@${docker_registry_image.app.sha256_digest}"
  container_port     = var.container_port
  subnet_ids         = module.network.subnet_ids
  security_group_ids = [module.network.security_group_id]
  execution_role_arn = data.aws_iam_role.lab_role.arn
  task_role_arn      = data.aws_iam_role.lab_role.arn
  log_group_name     = module.logging.log_group_name
  ecs_count          = var.horizontal_scaling ? var.min_capacity : var.ecs_count
  region             = var.aws_region
  cpu                = var.task_cpu
  memory             = var.task_memory
  target_group_arn   = var.horizontal_scaling ? module.alb[0].target_group_arn : null
}

module "autoscaling" {
  source             = "./modules/autoscaling"
  count              = var.horizontal_scaling ? 1 : 0
  cluster_name       = module.ecs.cluster_name
  service_name       = module.ecs.service_name
  min_capacity       = var.min_capacity
  max_capacity       = var.max_capacity
  target_cpu         = var.target_cpu
  scale_out_cooldown = var.scale_out_cooldown
  scale_in_cooldown  = var.scale_in_cooldown
}

module "dashboard" {
  source                  = "./modules/dashboard"
  region                  = var.aws_region
  cluster_name            = module.ecs.cluster_name
  service_name            = module.ecs.service_name
  task_cpu                = tonumber(var.task_cpu)
  target_cpu              = var.target_cpu
  alb_arn_suffix          = var.horizontal_scaling ? module.alb[0].arn_suffix : null
  target_group_arn_suffix = var.horizontal_scaling ? module.alb[0].target_group_arn_suffix : null
}

resource "docker_image" "app" {
  name = "${module.ecr.repository_url}:latest"

  build {
    context = "../src"
  }

  triggers = {
    src_hash = sha1(join("", [for f in sort(fileset("${path.module}/../src", "**")) : filesha1("${path.module}/../src/${f}")]))
  }
}

resource "docker_registry_image" "app" {
  name = docker_image.app.name
}
