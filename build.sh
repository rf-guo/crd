#!/usr/bin/env bash
echo "GOOS=linux go build -o ./app ."
GOOS=linux go build -o ./app .

VERSION=1.0.1

#docker build -t harbor.finupgroup.com/decisionoctopus/decisiontrain:${VERSION} .
#docker push harbor.finupgroup.com/decisionoctopus/decisiontrain:${VERSION}

docker build -t 123.59.150.220/decision/decisiontrain:${VERSION} .
docker push 123.59.150.220/decision/decisiontrain:${VERSION}
