locals {
  src_hash = sha1(join("", [for f in sort(fileset("${path.module}/../src", "**")) : filesha1("${path.module}/../src/${f}")]))
}

resource "aws_ecr_repository" "cart" {
  name         = "${var.name}-cart-service"
  force_delete = true
}

resource "docker_image" "cart" {
  name = "${aws_ecr_repository.cart.repository_url}:latest"
  build {
    context = "${path.module}/../src"
  }
  triggers = { src_hash = local.src_hash }
}

resource "docker_registry_image" "cart" {
  name     = docker_image.cart.name
  triggers = { image_id = docker_image.cart.latest }
}

resource "aws_cloudwatch_log_group" "cart" {
  name              = "/ecs/${var.name}-cart-service"
  retention_in_days = var.log_retention_days
}

resource "aws_ecs_cluster" "main" {
  name = "${var.name}-cluster"
}

resource "aws_ecs_task_definition" "cart" {
  family                   = "${var.name}-cart-service"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }
  container_definitions = jsonencode([{
    name         = "cart-service"
    image        = "${aws_ecr_repository.cart.repository_url}@${docker_registry_image.cart.sha256_digest}"
    essential    = true
    portMappings = [{ containerPort = 8080 }]
    environment = [
      { name = "TABLE_NAME", value = aws_dynamodb_table.carts.name },
      { name = "CUSTOMER_INDEX", value = "customer_id-index" },
      { name = "AWS_REGION", value = var.aws_region },
      { name = "ENABLE_DEBUG", value = "true" }, # consistency-probe endpoint for Part 3
      { name = "HTTP_MAX_IDLE_PER_HOST", value = tostring(var.max_idle_conns_per_host) },
      { name = "CART_BACKEND", value = var.database },
      { name = "DB_ADDR", value = aws_db_instance.mysql.endpoint }, # already host:port
      { name = "DB_NAME", value = aws_db_instance.mysql.db_name },
      { name = "DB_USER", value = aws_db_instance.mysql.username },
      { name = "DB_PASSWORD", value = random_password.mysql.result },
      { name = "POOL_SIZE", value = tostring(var.mysql_pool_size) },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.cart.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "cart" {
  name            = "${var.name}-cart-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.cart.arn
  desired_count   = var.task_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.tasks.id]
    assign_public_ip = true
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.cart.arn
    container_name   = "cart-service"
    container_port   = 8080
  }
  wait_for_steady_state = true # tests never hit old tasks after a change (the problem Eason and Shikha hit)
  depends_on            = [aws_lb_listener.http]
}
