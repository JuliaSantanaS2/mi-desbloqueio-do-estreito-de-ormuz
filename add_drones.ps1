# Script para adicionar 3 novos drones ao sistema (A2, B2 e C2)
# Certifique-se de estar na raiz do projeto ao rodar este script.
# Como rodar no Windows: .\add_drones.ps1

$PEER_BASES = "localhost:8001,localhost:8011,localhost:8021,localhost:8031"
$HB_TARGETS = "localhost:8500,localhost:8510,localhost:8520,localhost:8530"

Write-Host "Iniciando Drone A2 (Setor A)..."
Start-Process cmd -ArgumentList "/c title Drone A2 && set DRONE_ID=drone-A2&& set SECTOR_ID=A&& set DRONE_ADDR=localhost:9101&& set BASE_ADDR=localhost:8001&& set PEER_BASES=$PEER_BASES&& set HB_TARGETS=$HB_TARGETS&& go run cmd/drone/main.go"

Write-Host "Iniciando Drone B2 (Setor B)..."
Start-Process cmd -ArgumentList "/c title Drone B2 && set DRONE_ID=drone-B2&& set SECTOR_ID=B&& set DRONE_ADDR=localhost:9111&& set BASE_ADDR=localhost:8011&& set PEER_BASES=$PEER_BASES&& set HB_TARGETS=$HB_TARGETS&& go run cmd/drone/main.go"

Write-Host "Iniciando Drone C2 (Setor C)..."
Start-Process cmd -ArgumentList "/c title Drone C2 && set DRONE_ID=drone-C2&& set SECTOR_ID=C&& set DRONE_ADDR=localhost:9121&& set BASE_ADDR=localhost:8021&& set PEER_BASES=$PEER_BASES&& set HB_TARGETS=$HB_TARGETS&& go run cmd/drone/main.go"

Write-Host "🚀 3 Drones (A2, B2, C2) enviados para a rede!"