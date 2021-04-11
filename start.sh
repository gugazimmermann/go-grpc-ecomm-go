#!/bin/bash

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

exit 0
