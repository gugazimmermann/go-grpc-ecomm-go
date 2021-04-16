#!/bin/bash
cat banner.txt
RED='\033[0;31m'
NC='\033[0m'

docker-compose -f docker/keycloak/docker-compose.yml stop
if [ $? -ne 0 ]; then
  printf "${RED}Failed to stop keycloak container${NC}\n"
  exit 1
fi

docker-compose -f docker/mongo/docker-compose.yml stop
if [ $? -ne 0 ]; then
  printf "${RED}Failed to stop mongo container${NC}\n"
  exit 1
fi

docker-compose -f docker/go/docker-compose.yml stop
if [ $? -ne 0 ]; then
  printf "${RED}Failed to stop go server container${NC}\n"
  exit 1
fi

docker-compose -f docker/nginx/docker-compose.yml stop
if [ $? -ne 0 ]; then
  printf "${RED}Failed to stop nginx container${NC}\n"
  exit 1
fi

