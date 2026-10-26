#!/usr/bin/env bash
# Writes the public IP of each running ECS task to a Prometheus file_sd file.
set -u
OUT=${OUT:-/sd/targets.json}
INTERVAL=${INTERVAL:-10}
PORT=${PORT:-8080}

while true; do
  arns=$(aws ecs list-tasks --cluster "$CLUSTER" --service-name "$SERVICE" \
    --desired-status RUNNING --query 'taskArns' --output text 2>&1)
  if [[ $? -ne 0 ]]; then
    echo "list-tasks failed (expired Learner Lab credentials?): $arns" >&2
    sleep "$INTERVAL"; continue
  fi

  declare -A eni_of=()
  if [[ -n "$arns" && "$arns" != "None" ]]; then
    while read -r arn eni; do
      [[ -n "$eni" && "$eni" != "None" ]] && eni_of[${arn##*/}]=$eni
    done < <(aws ecs describe-tasks --cluster "$CLUSTER" --tasks $arns \
      --query "tasks[?lastStatus=='RUNNING'].[taskArn, attachments[0].details[?name=='networkInterfaceId'].value | [0]]" \
      --output text)
  fi

  declare -A ip_of=()
  if (( ${#eni_of[@]} > 0 )); then
    while read -r eni ip; do
      ip_of[$eni]=$ip
    done < <(aws ec2 describe-network-interfaces --network-interface-ids "${eni_of[@]}" \
      --query 'NetworkInterfaces[].[NetworkInterfaceId, Association.PublicIp]' --output text)
  fi

  json="["
  for task in "${!eni_of[@]}"; do
    ip=${ip_of[${eni_of[$task]}]:-}
    [[ -z "$ip" || "$ip" == "None" ]] && continue
    [[ "$json" != "[" ]] && json+=","
    json+="{\"targets\":[\"$ip:$PORT\"],\"labels\":{\"task\":\"${task:0:8}\"}}"
  done
  json+="]"

  echo "$json" > "$OUT.tmp" && mv "$OUT.tmp" "$OUT"
  echo "$(date -u +%H:%M:%S) targets: $json"
  unset eni_of ip_of
  sleep "$INTERVAL"
done
