resource "aws_lb" "main" {
  name               = "${var.name}-alb"
  load_balancer_type = "application"
  internal           = false
  subnets            = aws_subnet.public[*].id
  security_groups    = [aws_security_group.alb.id]
  idle_timeout       = 60 # sync requests queued longer than this get a 504
}

resource "aws_lb_target_group" "receiver" {
  name                 = "${var.name}-receiver-tg"
  target_type          = "ip"
  protocol             = "HTTP"
  port                 = 8080
  vpc_id               = aws_vpc.main.id
  deregistration_delay = 10

  health_check {
    path                = "/health"
    matcher             = "200"
    interval            = 10
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.receiver.arn
  }
}
