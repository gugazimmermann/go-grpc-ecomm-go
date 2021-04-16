#!/bin/bash
cat banner.txt
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

if [[ ! -f .env ]]; then

read -p "Type the USERNAME [go_user]: " MONGO_USERNAME
MONGO_USERNAME=${MONGO_USERNAME:-go_user}
echo $MONGO_USERNAME

read -p "Type the PASSWORD [go_pwd]: " MONGO_PASSWORD
MONGO_PASSWORD=${MONGO_PASSWORD:-go_pwd}
echo $MONGO_PASSWORD

MONGO_DB=gogrpcecomm

if [[ -z "${MONGO_USERNAME}" || -z "${MONGO_PASSWORD}" || -z "${MONGO_DB}" ]]; then
    echo "required inputs misssing"
    exit 1
fi

echo "CREATING .env FILE..."
cat >.env <<EOF
MONGO_USERNAME=${MONGO_USERNAME}
MONGO_PASSWORD=${MONGO_PASSWORD}
MONGO_DB=${MONGO_DB}
KEYCLOAK_URL=http://localhost:8082/auth/realms/go-grpc-ecomm-react/protocol/openid-connect/userinfo
EOF
echo "created..."

echo "CREATING init-mongo.sh FILE..."
cat >init-mongo.sh <<EOF
#!/usr/bin/env bash

echo 'Creating application user and db';

mongo ${MONGO_DB} \
--username ${MONGO_USERNAME} \
--password ${MONGO_PASSWORD} \
--authenticationDatabase admin \
--host localhost \
--port 27017 \
--eval "db.createUser({user: '${MONGO_USERNAME}', pwd: '${MONGO_PASSWORD}', roles:[{role:'dbOwner', db: '${MONGO_DB}'}]});"

echo 'User: ${MONGO_USERNAME} create to database ${MONGO_DB}';

EOF
echo "created..."

fi

printf "${GREEN}Starting keycloak${NC}\n"
{
  docker-compose -f docker/keycloak/docker-compose.yml build --no-cache 
  docker-compose -f docker/keycloak/docker-compose.yml up -d 
} > /dev/null
if [ $? -ne 0 ]; then
  printf "${RED}Keycloak failed${NC}\n"
  exit 1
fi

printf "${GREEN}Starting MongoDB${NC}\n"
{
  docker-compose -f docker/mongo/docker-compose.yml build --no-cache 
  docker-compose -f docker/mongo/docker-compose.yml up -d 
} > /dev/null
if [ $? -ne 0 ]; then
  printf "${RED}MongoDB failed${NC}\n"
  exit 1
fi

printf "${GREEN}Starting Go Server${NC}\n"
{
  docker-compose -f docker/go/docker-compose.yml build --no-cache 
  docker-compose -f docker/go/docker-compose.yml up -d 
} > /dev/null
if [ $? -ne 0 ]; then
  printf "${RED}Go Server failed${NC}\n"
  exit 1
fi

printf "${GREEN}Starting Nginx server${NC}\n"
{
  docker-compose -f docker/nginx/docker-compose.yml build --no-cache 
  docker-compose -f docker/nginx/docker-compose.yml up -d 
} > /dev/null
if [ $? -ne 0 ]; then
  printf "${RED}Nginx Failed${NC}\n"
  exit 1
fi

tries=0
until curl -s -o /dev/null localhost; do
  printf "Waiting server start...\n"
  sleep 5
  tries=$(($tries + 1))
  if [ $tries -ge 12 ]; then
    printf "${RED}server is not running:${NC}\n"
    exit 1
  fi
done
