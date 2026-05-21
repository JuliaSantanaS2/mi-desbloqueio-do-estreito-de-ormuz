#!/bin/bash

# ================================================================
#  Drone System - Maquina 1 (Setor A)
# ================================================================

# IPs das maquinas
export M1_IP="172.16.103.2"
export M2_IP="172.16.103.3"

# Definindo o nome do projeto para evitar erro de nome de imagem do Docker
export COMPOSE_PROJECT_NAME="drone_m1"

echo ""
echo " ================================================"
echo "  Iniciando Maquina 1 (Setor A) - IP Local: $M1_IP"
echo "  Conectando com Maquina 2: $M2_IP"
echo " ================================================"
echo ""

if ! docker info > /dev/null 2>&1; then
    echo " [ERRO] Docker não encontrado ou não esta rodando."
    exit 1
fi

echo " Parando containers anteriores..."
docker-compose -f docker-compose.m1.yml down > /dev/null 2>&1
sleep 1

echo " Subindo containers da Maquina 1 em background..."
docker-compose -f docker-compose.m1.yml up --build -d

echo ""
echo " [OK] Sistema rodando!"
echo " Para ver os logs interativamente, use:"
echo "   docker-compose -f docker-compose.m1.yml logs -f"
echo ""
echo " Ou acesse o Web Dashboard no navegador (a partir de qualquer PC na rede):"
echo "   http://$M1_IP:8080"
echo ""
echo " Para parar o sistema:"
echo "   docker-compose -f docker-compose.m1.yml down"
