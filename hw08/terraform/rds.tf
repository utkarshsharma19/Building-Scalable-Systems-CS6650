# STEP I: Amazon RDS MySQL 8.0 on db.t3.micro, in private subnets, reachable only from the ECS tasks.
# The private subnets have no route to the internet at all (no NAT needed: only the tasks talk to RDS,
# inside the VPC).

resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(aws_vpc.main.cidr_block, 8, count.index + 10)
  availability_zone = data.aws_availability_zones.available.names[count.index]
  tags              = { Name = "${var.name}-private-${count.index + 1}" }
}

resource "aws_db_subnet_group" "mysql" {
  name       = "${var.name}-mysql"
  subnet_ids = aws_subnet.private[*].id
}

resource "aws_security_group" "mysql" {
  name        = "${var.name}-mysql-sg"
  description = "3306 from the ECS tasks only"
  vpc_id      = aws_vpc.main.id
  ingress {
    from_port       = 3306
    to_port         = 3306
    protocol        = "tcp"
    security_groups = [aws_security_group.tasks.id]
  }
}

resource "random_password" "mysql" {
  length  = 24
  special = false
}

resource "aws_db_instance" "mysql" {
  identifier              = "${var.name}-mysql"
  engine                  = "mysql"
  engine_version          = "8.0"
  instance_class          = "db.t3.micro" # free-tier class
  allocated_storage       = 20
  storage_type            = "gp2"
  db_name                 = "carts"
  username                = "cartadmin"
  password                = random_password.mysql.result
  db_subnet_group_name    = aws_db_subnet_group.mysql.name
  vpc_security_group_ids  = [aws_security_group.mysql.id]
  publicly_accessible     = false
  multi_az                = false
  backup_retention_period = 0
  skip_final_snapshot     = true  # assignment: destroy must work right away
  deletion_protection     = false # assignment
  apply_immediately       = true
}

output "mysql_endpoint" {
  value = aws_db_instance.mysql.endpoint
}
