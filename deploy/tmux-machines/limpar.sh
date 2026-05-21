#!/bin/bash
echo "Limpando processos e containers..."
pkill xterm
docker-compose down --remove-orphans
docker rm -f $(docker ps -aq) 2>/dev/null
docker network prune -f
echo "Ambiente limpo!"