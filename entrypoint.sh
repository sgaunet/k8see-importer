#!/usr/bin/env bash

function checkVarIsNotEmpty
{
  var="$1"
  eval "value=\$$var"
  if [ -z "$value" ]
  then  
    echo "ERROR: $var not set. EXIT 1"
    exit 1
  fi
}

checkVarIsNotEmpty REDIS_HOST
checkVarIsNotEmpty REDIS_STREAM
checkVarIsNotEmpty DBHOST
checkVarIsNotEmpty DBNAME
checkVarIsNotEmpty DBUSER
checkVarIsNotEmpty DBPASSWORD

echo "redis_host: $REDIS_HOST"          > /opt/k8see-importer/conf.yaml
echo "redis_port: ${REDIS_PORT:-6379}"  >> /opt/k8see-importer/conf.yaml
echo "redis_password: $REDIS_PASSWORD" >> /opt/k8see-importer/conf.yaml
echo "redis_stream: $REDIS_STREAM"     >> /opt/k8see-importer/conf.yaml
echo "dbhost: $DBHOST"                 >> /opt/k8see-importer/conf.yaml
echo "dbport: ${DBPORT:-5432}"          >> /opt/k8see-importer/conf.yaml
echo "dbname: $DBNAME"                 >> /opt/k8see-importer/conf.yaml
echo "dbuser: $DBUSER"                 >> /opt/k8see-importer/conf.yaml
echo "dbpassword: $DBPASSWORD"         >> /opt/k8see-importer/conf.yaml
echo "loglevel: ${LOGLEVEL:warn}"      >> /opt/k8see-importer/conf.yaml

export DATABASE_URL="postgres://${DBUSER}:${DBPASSWORD}@${DBHOST}:${DBPORT}/${DBNAME}?sslmode=disable"

echo "INFO: Wait for database connection"
dbmate wait
echo "INFO: Database is ready"

echo "INFO: create the database (if it does not already exist) and run any pending migrations"
dbmate up
rc=$?

if [ "$rc" = "0" ]
then
  echo "INFO: create tables OK"
else
  echo "ERROR: an error occured. exit 1"
  exit 1
fi

exec $@
