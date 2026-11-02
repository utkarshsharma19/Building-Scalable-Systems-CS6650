# Source this file:  source midterm-mastery/part2/scripts/env.sh
# Points every experiment script at the forked service repo and its live Terraform outputs.
export AWS_REGION=${AWS_REGION:-us-west-2}
export AWS_DEFAULT_REGION=$AWS_REGION
PART2=$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/.." && pwd)
export PART2
export SERVICE_REPO=${SERVICE_REPO:-$HOME/Desktop/Scabale-System-Go/file-transfer-playservice}
export VENV=${VENV:-$HOME/Desktop/Scabale-System-Go/.venv-midterm}
export RESULTS=$PART2/results
export CLUSTER=file-transfer
mkdir -p "$RESULTS"

if terraform -chdir="$SERVICE_REPO/terraform" output -raw alb_dns_name >/dev/null 2>&1; then
	export GATEWAY="http://$(terraform -chdir="$SERVICE_REPO/terraform" output -raw alb_dns_name)"
	export BUCKET=$(terraform -chdir="$SERVICE_REPO/terraform" output -raw files_bucket)
	export PREFIX=$(terraform -chdir="$SERVICE_REPO/terraform" output -raw files_prefix)
	echo "GATEWAY=$GATEWAY  BUCKET=s3://$BUCKET/$PREFIX"
else
	echo "(stack not deployed yet: GATEWAY is unset)"
fi
