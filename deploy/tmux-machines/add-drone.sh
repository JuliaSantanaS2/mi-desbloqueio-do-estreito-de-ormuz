#!/bin/bash

# ================================================================
#  Drone System - Adicionador Dinâmico de Drones
# ================================================================

MACHINE_ID=$1
DRONE_NUM=$2

if [[ -z "$MACHINE_ID" || -z "$DRONE_NUM" ]]; then
    echo "Uso: ./add-drone.sh <numero_da_maquina> <numero_do_drone>"
    echo "Exemplo: ./add-drone.sh 1 2"
    exit 1
fi

# IPs Fixos do Laboratório
IP_1="172.16.103.1"
IP_2="172.16.103.2"
IP_3="172.16.103.3"
IP_4="172.16.103.4"

# Letras MAIÚSCULAS para não confundir o sistema em Go!
case $MACHINE_ID in
    1) export SECTOR_ID="A"; LOCAL_IP=$IP_1; PEERS=("$IP_2" "$IP_3" "$IP_4") ;;
    2) export SECTOR_ID="B"; LOCAL_IP=$IP_2; PEERS=("$IP_1" "$IP_3" "$IP_4") ;;
    3) export SECTOR_ID="C"; LOCAL_IP=$IP_3; PEERS=("$IP_1" "$IP_2" "$IP_4") ;;
    4) export SECTOR_ID="D"; LOCAL_IP=$IP_4; PEERS=("$IP_1" "$IP_2" "$IP_3") ;;
    *) echo "ID de máquina inválido (1-4)"; exit 1 ;;
esac

export PEER_BASES_8101="${PEERS[0]}:8101,${PEERS[1]}:8101,${PEERS[2]}:8101"
export PEER_HB_8550="${PEERS[0]}:8550,${PEERS[1]}:8550,${PEERS[2]}:8550"

DRONE_PORT=$((9100 + DRONE_NUM))
DRONE_ID="drone-${SECTOR_ID}${DRONE_NUM}"

# O nome da imagem criada pelo tmux-machines é esse:
IMAGE_NAME="tmux-machines-drone"

echo "🚀 Abrindo XTerm para o Drone: $DRONE_ID na porta $DRONE_PORT"

xterm -T "Drone $DRONE_ID - Setor $SECTOR_ID" -geometry 80x20 \
    -e "docker run --rm --name \"$DRONE_ID\" --network host \
    -e DRONE_ID=\"$DRONE_ID\" \
    -e SECTOR_ID=\"${SECTOR_ID}\" \
    -e DRONE_ADDR=\"0.0.0.0:${DRONE_PORT}\" \
    -e DRONE_HOST=\"${LOCAL_IP}:${DRONE_PORT}\" \
    -e BASE_ADDR=\"127.0.0.1:8101\" \
    -e PEER_BASES=\"${PEER_BASES_8101}\" \
    -e HB_TARGETS=\"127.0.0.1:8550,${PEER_HB_8550}\" \
    $IMAGE_NAME" &