# run_local.ps1 - Executa o sistema de drones SEM Docker
#
# Uso:
#   powershell -ExecutionPolicy Bypass -File .\run_local.ps1
#
# Para parar tudo:
#   powershell -ExecutionPolicy Bypass -File .\run_local.ps1 -Stop

param(
    [switch]$Stop,
    [switch]$BuildOnly
)

$ROOT = $PSScriptRoot
Set-Location -Path $ROOT

# Parar todos os processos do sistema
if ($Stop) {
    Write-Host "Parando todos os processos do drone-system..." -ForegroundColor Red
    $procs = Get-Process powershell -ErrorAction SilentlyContinue | Where-Object {
        try { $_.MainWindowTitle -match "DRONE-SYSTEM" } catch { $false }
    }
    if ($procs) {
        $procs | Stop-Process -Force -ErrorAction SilentlyContinue
        Write-Host "  $($procs.Count) processo(s) encerrado(s)." -ForegroundColor Green
    } else {
        Write-Host "  Nenhum processo encontrado." -ForegroundColor Yellow
    }
    # Limpa scripts temporarios
    Remove-Item "$env:TEMP\dronesys_*.ps1" -ErrorAction SilentlyContinue
    exit 0
}

# ── Build ──────────────────────────────────────────────────
Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "  DRONE SYSTEM - Build local" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""

New-Item -ItemType Directory -Force -Path "$ROOT\bin" | Out-Null

$bins = @("broker", "base", "sensor", "drone", "dashboard", "webdash")
$buildOk = $true

foreach ($b in $bins) {
    Write-Host "Compilando $b ..." -NoNewline
    $result = & go build -o "$ROOT\bin\$b.exe" "$ROOT\cmd\$b" 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host " ERRO" -ForegroundColor Red
        Write-Host $result -ForegroundColor Red
        $buildOk = $false
    } else {
        Write-Host " OK" -ForegroundColor Green
    }
}

if (-not $buildOk) {
    Write-Host ""
    Write-Host "Build falhou. Corrija os erros antes de continuar." -ForegroundColor Red
    exit 1
}

if ($BuildOnly) {
    Write-Host "Build concluido. Binarios em $ROOT\bin\" -ForegroundColor Green
    exit 0
}

# ── Funcao: cria arquivo .ps1 temporario e abre em nova janela ──
# Usando arquivo temporario evita problemas de quoting com espacos no caminho.
function Start-Component {
    param(
        [string]$Title,
        [string]$ExePath,
        [hashtable]$EnvVars
    )

    # Monta as linhas de configuracao de env
    $lines = @()
    $lines += "`$host.UI.RawUI.WindowTitle = 'DRONE-SYSTEM $Title'"
    foreach ($kv in $EnvVars.GetEnumerator()) {
        $lines += "`$env:$($kv.Key) = '$($kv.Value)'"
    }
    $lines += "Write-Host ''"
    $lines += "Write-Host '  [ $Title ]  ' -BackgroundColor DarkBlue -ForegroundColor White"
    $lines += "Write-Host ''"
    $lines += "& `"$ExePath`""
    $lines += "Write-Host ''"
    $lines += "Write-Host 'Processo encerrado. Pressione qualquer tecla para fechar...' -ForegroundColor Yellow"
    $lines += "Read-Host"

    # Salva em arquivo temporario (sem espacos no caminho)
    $tmpFile = "$env:TEMP\dronesys_$Title.ps1"
    $lines | Set-Content -Path $tmpFile -Encoding UTF8

    Start-Process powershell -ArgumentList "-NoExit", "-ExecutionPolicy", "Bypass", "-File", $tmpFile
    Start-Sleep -Milliseconds 400
}

# ── Configuracao dos setores ───────────────────────────────
Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "  Iniciando componentes..." -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""

# Arquitetura: 4 setores, cada um com 1 broker + 1 base + 1 drone + 2 sensores
# Sensores com intervalo 5s para ritmo de testes
$sectors = @(
    [ordered]@{ ID="A"; BPort=7001; BasePort=8001; GossipPort=8002; HBPort=8500; DashPort=3001; DronePort=9001; SI=@(5,5) }
    [ordered]@{ ID="B"; BPort=7011; BasePort=8011; GossipPort=8012; HBPort=8510; DashPort=3011; DronePort=9011; SI=@(5,5) }
    [ordered]@{ ID="C"; BPort=7021; BasePort=8021; GossipPort=8022; HBPort=8520; DashPort=3021; DronePort=9021; SI=@(5,5) }
    [ordered]@{ ID="D"; BPort=7031; BasePort=8031; GossipPort=8032; HBPort=8530; DashPort=3031; DronePort=9031; SI=@(5,5) }
)

# Calcula peers para cada setor
foreach ($s in $sectors) {
    $others = $sectors | Where-Object { $_.ID -ne $s.ID }
    $s["PeerBases"]   = ($others | ForEach-Object { "localhost:$($_.BasePort)" }) -join ","
    $s["PeerGossip"]  = ($others | ForEach-Object { "localhost:$($_.GossipPort)" }) -join ","
    $s["PeerHB"]      = ($others | ForEach-Object { "localhost:$($_.HBPort)" }) -join ","
    $s["PeerBrokers"] = ($others | ForEach-Object { "localhost:$($_.BPort)" }) -join ","
    $s["HBTargets"]   = ($sectors | ForEach-Object { "localhost:$($_.HBPort)" }) -join ","
}

# ── Fase 1: Brokers ────────────────────────────────────────
Write-Host "[ 1/4 ] Iniciando Brokers..." -ForegroundColor Yellow
foreach ($s in $sectors) {
    Start-Component -Title "broker-$($s.ID)" -ExePath "$ROOT\bin\broker.exe" -EnvVars @{
        SECTOR_ID   = $s.ID
        BROKER_ADDR = "0.0.0.0:$($s.BPort)"
        PEER_BASES  = $s.PeerBases
    }
    Write-Host "  broker-$($s.ID) -> localhost:$($s.BPort)" -ForegroundColor Green
}
Write-Host "Aguardando brokers (2s)..." -ForegroundColor Gray
Start-Sleep -Seconds 2

# ── Fase 2: Bases ──────────────────────────────────────────
Write-Host "[ 2/4 ] Iniciando Bases..." -ForegroundColor Yellow
foreach ($s in $sectors) {
    Start-Component -Title "base-$($s.ID)" -ExePath "$ROOT\bin\base.exe" -EnvVars @{
        SECTOR_ID   = $s.ID
        BASE_ADDR   = "0.0.0.0:$($s.BasePort)"
        GOSSIP_ADDR = "0.0.0.0:$($s.GossipPort)"
        HB_ADDR     = "0.0.0.0:$($s.HBPort)"
        DASH_ADDR   = "0.0.0.0:$($s.DashPort)"
        PEER_BASES  = $s.PeerBases
        PEER_GOSSIP = $s.PeerGossip
        PEER_HB     = $s.PeerHB
    }
    Write-Host "  base-$($s.ID) -> :$($s.BasePort) gossip:$($s.GossipPort) hb:$($s.HBPort) dash:$($s.DashPort)" -ForegroundColor Green
}
Write-Host "Aguardando bases (2s)..." -ForegroundColor Gray
Start-Sleep -Seconds 2

# ── Fase 3: Drones ─────────────────────────────────────────
Write-Host "[ 3/4 ] Iniciando Drones..." -ForegroundColor Yellow
foreach ($s in $sectors) {
    Start-Component -Title "drone-$($s.ID)1" -ExePath "$ROOT\bin\drone.exe" -EnvVars @{
        DRONE_ID   = "drone-$($s.ID)1"
        SECTOR_ID  = $s.ID
        DRONE_ADDR = "0.0.0.0:$($s.DronePort)"
        BASE_ADDR  = "localhost:$($s.BasePort)"
        PEER_BASES = $s.PeerBases
        HB_TARGETS = $s.HBTargets
    }
    Write-Host "  drone-$($s.ID)1 -> base localhost:$($s.BasePort)" -ForegroundColor Green
}
Write-Host "Aguardando drones (2s)..." -ForegroundColor Gray
Start-Sleep -Seconds 2

# ── Fase 4: Sensores ───────────────────────────────────────
Write-Host "[ 4/4 ] Iniciando Sensores..." -ForegroundColor Yellow
foreach ($s in $sectors) {
    $i = 1
    foreach ($interval in $s.SI) {
        Start-Component -Title "sensor-$($s.ID)$i" -ExePath "$ROOT\bin\sensor.exe" -EnvVars @{
            SECTOR_ID    = $s.ID
            SENSOR_ID    = "sensor-$($s.ID)$i"
            BROKER_ADDR  = "localhost:$($s.BPort)"
            PEER_BROKERS = $s.PeerBrokers
            INTERVAL     = "$interval"
        }
        Write-Host "  sensor-$($s.ID)$i -> broker :$($s.BPort) intervalo:${interval}s" -ForegroundColor Green
        $i++
    }
}

Start-Sleep -Seconds 1

# ── Dashboard TUI ──────────────────────────────────────────
Write-Host ""
Write-Host "Iniciando Dashboard TUI (Setor A - porta 3001)..." -ForegroundColor Magenta
Start-Component -Title "dashboard-A" -ExePath "$ROOT\bin\dashboard.exe" -EnvVars @{
    SECTOR_ID      = "A"
    BASE_DASH_ADDR = "localhost:3001"
}

Start-Sleep -Seconds 1

# ── Web Dashboard ──────────────────────────────────────────
Write-Host ""
Write-Host "Iniciando Web Dashboard (http://localhost:8080)..." -ForegroundColor Magenta
Start-Component -Title "webdash" -ExePath "$ROOT\bin\webdash.exe" -EnvVars @{
    HTTP_ADDR  = ":8080"
    BASE_ADDRS = "A=localhost:3001,B=localhost:3011,C=localhost:3021,D=localhost:3031"
    BASE_TCP_ADDRS = "A=localhost:8001,B=localhost:8011,C=localhost:8021,D=localhost:8031"
}

Start-Sleep -Seconds 2

# Abre o browser automaticamente
Write-Host "  Abrindo browser em http://localhost:8080 ..." -ForegroundColor Cyan
Start-Process "http://localhost:8080"

# ── Resumo ─────────────────────────────────────────────────
Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "  Sistema iniciado com sucesso!" -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Janelas abertas:" -ForegroundColor White
Write-Host "  4 Brokers  (A:7001  B:7011  C:7021  D:7031)" -ForegroundColor Gray
Write-Host "  4 Bases    (A:8001  B:8011  C:8021  D:8031)" -ForegroundColor Gray
Write-Host "  4 Drones   (A:9001  B:9011  C:9021  D:9031)" -ForegroundColor Gray
Write-Host "  8 Sensores (2 por setor)" -ForegroundColor Gray
Write-Host "  1 Dashboard TUI (Setor A - porta 3001)" -ForegroundColor Gray
Write-Host "  1 Web Dashboard  -> http://localhost:8080" -ForegroundColor Cyan
Write-Host ""
Write-Host ""
Write-Host "Para adicionar mais drones para testar o auto-balanceamento:" -ForegroundColor Yellow
Write-Host "  Abra um NOVO terminal PowerShell e rode o comando abaixo:" -ForegroundColor White
Write-Host '  powershell -ExecutionPolicy Bypass -Command "& { $env:DRONE_ID=''drone-EXTRA''; $env:SECTOR_ID=''A''; $env:DRONE_ADDR=''0.0.0.0:9099''; $env:BASE_ADDR=''localhost:8001''; $env:PEER_BASES=''localhost:8011,localhost:8021,localhost:8031''; $env:HB_TARGETS=''localhost:8500,localhost:8510,localhost:8520,localhost:8530''; .\bin\drone.exe }"' -ForegroundColor Cyan
Write-Host ""
Write-Host "Para parar tudo:" -ForegroundColor Yellow
Write-Host "  powershell -ExecutionPolicy Bypass -File .\run_local.ps1 -Stop" -ForegroundColor Yellow
Write-Host ""
