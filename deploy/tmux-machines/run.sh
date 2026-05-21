#!/bin/bash

if [ -z "$1" ]; then
  echo "Uso: ./run.sh [1|2|3|4]"
  exit 1
fi

MACHINE=$1

# IPs Fixos do Laboratório
IP1="172.16.103.1"
IP2="172.16.103.2"
IP3="172.16.103.3"
IP4="172.16.103.4"

# ==========================================
# VARIÁVEIS GLOBAIS DO WEBDASH (Mesmas para todos)
# ==========================================
export WEBDASH_BASES="A=$IP1:3101,B=$IP2:3101,C=$IP3:3101,D=$IP4:3101"
export WEBDASH_TCP_BASES="A=$IP1:8101,B=$IP2:8101,C=$IP3:8101,D=$IP4:8101"

# Mapeamento 
if [ "$MACHINE" == "1" ]; then
    export LOCAL_IP=$IP1
    export SECTOR_ID="A"
    export PEER_BASES="$IP2:8101,$IP3:8101,$IP4:8101"
    export PEER_GOSSIP="$IP2:8102,$IP3:8102,$IP4:8102"
    export PEER_HB="$IP2:8550,$IP3:8550,$IP4:8550"
elif [ "$MACHINE" == "2" ]; then
    export LOCAL_IP=$IP2
    export SECTOR_ID="B"
    export PEER_BASES="$IP1:8101,$IP3:8101,$IP4:8101"
    export PEER_GOSSIP="$IP1:8102,$IP3:8102,$IP4:8102"
    export PEER_HB="$IP1:8550,$IP3:8550,$IP4:8550"
elif [ "$MACHINE" == "3" ]; then
    export LOCAL_IP=$IP3
    export SECTOR_ID="C"
    export PEER_BASES="$IP1:8101,$IP2:8101,$IP4:8101"
    export PEER_GOSSIP="$IP1:8102,$IP2:8102,$IP4:8102"
    export PEER_HB="$IP1:8550,$IP2:8550,$IP4:8550"
elif [ "$MACHINE" == "4" ]; then
    export LOCAL_IP=$IP4
    export SECTOR_ID="D"
    export PEER_BASES="$IP1:8101,$IP2:8101,$IP3:8101"
    export PEER_GOSSIP="$IP1:8102,$IP2:8102,$IP3:8102"
    export PEER_HB="$IP1:8550,$IP2:8550,$IP3:8550"
else
    echo "Máquina inválida. Use 1, 2, 3 ou 4."
    exit 1
fi

echo "🚀 Iniciando MÁQUINA $MACHINE (Setor $SECTOR_ID | IP: $LOCAL_IP)..."

# Derruba o antigo e sobe o novo no background (-d)
docker compose down
docker compose up -d --build

# Dá 2 segundinhos pro docker levantar os containers
sleep 2

# ==========================================
# ABRE AS JANELAS DO XTERM
# ==========================================
xterm -T "DASHBOARD TUI - Setor $SECTOR_ID" -geometry 100x30 -e "docker compose attach dashboard" &
xterm -T "BASE - Setor $SECTOR_ID" -geometry 80x20 -e "docker compose logs -f base" &
xterm -T "DRONE - Setor $SECTOR_ID" -geometry 80x20 -e "docker compose logs -f drone" &
xterm -T "BROKER/SENSORS - Setor $SECTOR_ID" -geometry 80x20 -e "docker compose logs -f broker sensor" &
xterm -T "WEBDASH LOGS" -geometry 80x20 -e "docker compose logs -f webdash" &

echo "✅ Sistema no ar!"
echo "🌐 Acesse o Web Dashboard em: http://$LOCAL_IP:8180"