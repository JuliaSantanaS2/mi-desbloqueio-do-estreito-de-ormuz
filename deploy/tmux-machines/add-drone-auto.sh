#!/bin/bash
# ================================================================
#  Drone System - Adicionador Dinâmico UNIVERSAL (Auto-Routing)
# ================================================================

DRONE_NUM=$1

if [[ -z "$DRONE_NUM" ]]; then
    echo "Uso: ./add-drone-auto.sh <numero_do_drone>"
    echo "Exemplo: ./add-drone-auto.sh 99"
    exit 1
fi

# Detecta o IP real da máquina onde você está rodando o script agora
# Isso impede que o drone minta seu endereço para a Base!
LOCAL_IP=$(hostname -I | awk '{print $1}')
if [[ -z "$LOCAL_IP" ]]; then
    LOCAL_IP="127.0.0.1"
fi

# IPs Fixos do Laboratório
IP_A="172.16.103.1"
IP_B="172.16.103.2"
IP_C="172.16.103.3"
IP_D="172.16.103.4"

# Dizemos ao drone onde estão absolutamente todas as bases do cluster
ALL_BASES="${IP_A}:8101,${IP_B}:8101,${IP_C}:8101,${IP_D}:8101"
ALL_HBS="${IP_A}:8550,${IP_B}:8550,${IP_C}:8550,${IP_D}:8550"

DRONE_ID="drone-X${DRONE_NUM}"
DRONE_PORT=$((9200 + DRONE_NUM))

# O PULO DO GATO:
# Todos os drones entram no sistema pela porta da frente (Base A).
# O algoritmo de balanceamento matemático que fizemos no Go 
# vai olhar para a rede inteira e "chutar" o drone automaticamente
# para a B, C ou D caso a A não precise dele.
ENTRY_BASE="${IP_A}:8101"

echo "🚀 Lançando Drone Autônomo: $DRONE_ID na porta $DRONE_PORT (IP Real: $LOCAL_IP)"

xterm -T "Drone $DRONE_ID (Auto-Routing)" -geometry 80x20 \
    -e "docker run --rm --name \"$DRONE_ID\" --network host \
    -e DRONE_ID=\"$DRONE_ID\" \
    -e DRONE_ADDR=\"0.0.0.0:${DRONE_PORT}\" \
    -e DRONE_HOST=\"${LOCAL_IP}:${DRONE_PORT}\" \
    -e BASE_ADDR=\"${ENTRY_BASE}\" \
    -e PEER_BASES=\"${ALL_BASES}\" \
    -e HB_TARGETS=\"${ALL_HBS}\" \
    tmux-machines-drone" &