# 🛸 Sistema Distribuído de Drones — Monitoramento Marítimo no Estreito de Ormuz (PBL 2)

Este é um sistema distribuído desenvolvido em **Go** e conteinerizado com **Docker** para gerenciar e coordenar uma frota descentralizada de drones autônomos dedicados ao monitoramento marítimo no Estreito de Ormuz. 

A arquitetura emprega um modelo **P2P (Peer-to-Peer) Descentralizado**, garantindo a ausência de um ponto único de falha (Single Point of Failure - SPOF). A comunicação entre os nós da rede ocorre por meio dos protocolos **TCP** (para dados que exigem confiabilidade) e **UDP** (para detecção rápida de falhas por *heartbeats*), com suporte a sincronização via **Relógio Lógico de Lamport** e exclusão mútua distribuída com o algoritmo **Ricart-Agrawala**.

---

## 🏗️ Arquitetura da Solução

O sistema divide o Estreito de Ormuz em 4 setores geográficos (**A**, **B**, **C** e **D**). Cada setor possui componentes autônomos que cooperam de forma descentralizada:

```
                  ┌───────────────────────────────┐
                  │           Sensores            │
                  └───────────────┬───────────────┘
                                  │ (TCP/UDP)
                                  ▼
                  ┌───────────────────────────────┐
                  │            Brokers            │
                  └───────────────┬───────────────┘
                                  │ (TCP/UDP)
                                  ▼
                  ┌───────────────────────────────┐
                  │      Bases de Setor (P2P)     │◄───► [Outras Bases P2P]
                  └───────────────┬───────────────┘  (Ricart-Agrawala / Lamport)
                                  │ (TCP / Heartbeats UDP)
                                  ▼
                  ┌───────────────────────────────┐
                  │            Drones             │
                  └───────────────────────────────┘
```

*   **Bases de Setor (Nós P2P):** Formam o núcleo inteligente do sistema. Cada base mantém uma fila distribuída de requisições, gerencia a concorrência global (exclusão mútua) e despacha os drones disponíveis. Em caso de queda de uma base, as bases vizinhas assumem (herdam) automaticamente os seus drones ativos.
*   **Brokers:** Recebem os eventos de detecção emitidos pelos sensores de borda e realizam o roteamento dinâmico e seguro para as bases de setor ativas.
*   **Sensores:** Dispositivos na borda que geram ocorrências de tráfego marítimo com diferentes graus de criticidade (`CRITICAL` ou `NORMAL`).
*   **Drones:** Unidades executoras que respondem a comandos das bases de setor, voando até coordenadas `(X, Y)` específicas para monitoramento.
*   **Web Dashboard & TUI:** Interfaces que oferecem observabilidade em tempo real do estado de todos os setores (estado da fila, logs, alarmes, posições cartesianas dos drones e status das bases).

---

## 🛡️ Protocolo e Tratamento de Falhas

O sistema implementa tolerância a falhas baseada em timeouts, redundância e planos de contingência automáticos:

1.  **Deadlines e ACKs de Aplicação (TCP):** As conexões utilizam `SetReadDeadline` e `SetWriteDeadline` (definidos em [tcp.go](internal/network/tcp.go)) para evitar travar soquetes em nós travados. Além disso, a aplicação exige confirmações explícitas. Por exemplo, ao despachar um drone (`MsgDispatch`), a Base aguarda um `ACK` em formato de mensagem. Se não o receber dentro do tempo limite, o drone é considerado offline e a ocorrência é recolocada no topo da fila.
2.  **Detecção de Vitalidade por Heartbeats (UDP):** Drones e Bases emitem mensagens UDP de batimento cardíaco periódicas (definidos em [udp.go](internal/network/udp.go)). 
    *   Se uma **Base** deixa de receber heartbeats de um **Drone** por 9 segundos, ela o classifica como `LOST` e reatribui sua missão.
    *   Se as **Bases** vizinhas detectam a ausência de heartbeat de uma base específica por 9 segundos, elas ativam o **Plano de Contingência de Adoção de Drones**, redistribuindo a carga e adotando os drones órfãos da base caída.

---

## 🔒 Concorrência Distribuída e Sincronização

Para assegurar que múltiplas ocorrências não enviem múltiplos drones ao mesmo local (não-duplicidade) e evitar conflitos de despacho na rede P2P, aplicam-se duas estratégias de sincronização:

1.  **Ordenação de Eventos (Relógio Lógico de Lamport):** Cada evento de sensor e cada mensagem trafegada possui um carimbo de tempo lógico (*timestamp*). Em caso de empates na fila de prioridade, o nó com menor carimbo de Lamport (definido em [clock.go](internal/lamport/clock.go)) é atendido primeiro.
2.  **Exclusão Mútua Distribuída (Baseado no Algoritmo de Ricart-Agrawala):**
    *   Antes de despachar um drone para uma ocorrência pendente na fila local, a Base de Setor cria uma requisição de lock (`MsgLockRequest`) contendo o ID da ocorrência e seu timestamp de Lamport.
    *   Essa mensagem é transmitida para **todas as outras bases ativas** na rede.
    *   As bases receptoras avaliam a concorrência: se elas não estiverem competindo pela mesma ocorrência ou se tiverem uma requisição com timestamp de Lamport maior (mais recente), respondem concedendo o voto (`Granted = true`).
    *   O drone só é despachado se a base requisitante obtiver aprovação unânime de todas as bases que estão atualmente **online**. Se uma base falha ou perde conexão, o sistema a remove da lista de votação ativa, evitando que o algoritmo fique travado esperando a resposta de um nó inativo.

---

## 📂 Estrutura do Projeto

O código-fonte está estruturado da seguinte forma:

*   **[`cmd/`](cmd):** Contém os pontos de entrada do sistema.
    *   **[`cmd/base/`](cmd/base/main.go):** Código do nó P2P Base do Setor.
    *   **[`cmd/broker/`](cmd/broker/main.go):** Código do Broker de mensagens dos sensores.
    *   **[`cmd/dashboard/`](cmd/dashboard/main.go):** Console TUI interativo para visualização de dados de uma base local.
    *   **[`cmd/drone/`](cmd/drone/main.go):** Executável que emula os drones físicos.
    *   **[`cmd/sensor/`](cmd/sensor/main.go):** Emulador de sensores de detecção marítima.
    *   **[`cmd/testrunner/`](cmd/testrunner/main.go):** Suite de testes integrados e automatizados localmente em memória.
    *   **[`cmd/webdash/`](cmd/webdash/main.go):** Servidor HTTP e WebSocket que serve o Web Dashboard centralizado.
*   **[`internal/`](internal):** Lógica compartilhada do domínio.
    *   **[`internal/alerts/`](internal/alerts/alerts.go):** Mecanismo de notificação e níveis de alerta.
    *   **[`internal/lamport/`](internal/lamport/clock.go):** Implementação segura do Relógio Lógico de Lamport.
    *   **[`internal/models/`](internal/models/types.go):** Tipos de dados, mensagens e estruturas compartilhadas na rede.
    *   **[`internal/network/`](internal/network):** Utilitários de soquete TCP (`tcp.go`) e UDP (`udp.go`).
    *   **[`internal/queue/`](internal/queue/queue.go):** Fila de prioridade concorrente de requisições ordenada por criticidade e tempo lógico.
*   **[`deploy/`](deploy):** Arquivos de configuração de containers.
    *   `docker-compose.local.yml`: Roda toda a infraestrutura com 4 setores localmente no mesmo computador.
    *   `docker-compose.pcX.yml`: Configurações individuais para execução distribuída em 4 computadores físicos diferentes no laboratório.
    *   **[`deploy/tmux-machines/`](deploy/tmux-machines):** Scripts para provisionamento e monitoramento em ambiente Linux distribuído usando *tmux* e *xterm*.
*   **[`docker/`](docker):** Contém os Dockerfiles otimizados e multi-stage para cada componente do sistema.

---

## 🚀 Como Executar o Projeto

Existem três formas de executar a simulação do sistema distribuído:

### Opção A: Execução Local Nativa (Windows PowerShell)

Para desenvolvedores rodando em ambiente Windows sem necessidade de instalar o Docker:

1.  **Iniciar a Simulação Completa (Compila e abre janelas individuais):**
    ```powershell
    powershell -ExecutionPolicy Bypass -File .\run_local.ps1
    ```
    *Este comando irá compilar todos os binários na pasta `bin/` e abrir 16 janelas de terminal (4 bases, 4 brokers, 4 drones, 8 sensores, 1 dashboard TUI e o Web Dashboard).*

2.  **Adicionar Drones Extras (para testar balanceamento de carga):**
    ```powershell
    powershell -ExecutionPolicy Bypass -File .\add_drones.ps1
    ```
    *Cria três drones adicionais (`drone-A2`, `drone-B2` e `drone-C2`) para simular concorrência e fila concorrente.*

3.  **Parar a Simulação e Limpar Processos:**
    ```powershell
    powershell -ExecutionPolicy Bypass -File .\run_local.ps1 -Stop
    ```

---

### Opção B: Execução Local via Docker Compose

Caso possua o Docker e o Docker Compose instalados em sua máquina de desenvolvimento:

1.  **Subir todos os containers:**
    ```bash
    docker compose -f deploy/docker-compose.local.yml up -d --build
    ```
2.  **Visualizar logs de um componente específico (ex: Base A):**
    ```bash
    docker logs -f base-A
    ```
3.  **Parar a execução de todos os containers:**
    ```bash
    docker compose -f deploy/docker-compose.local.yml down
    ```

---

### Opção C: Execução Distribuída em Múltiplas Máquinas (Linux)

Usado para validar a execução distribuída real utilizando computadores na mesma rede local de laboratório (PC 1 a PC 4):

1.  Acesse o diretório do script:
    ```bash
    cd deploy/tmux-machines
    ```
2.  Execute o script passando o ID do computador físico (1, 2, 3 ou 4) correspondente:
    ```bash
    ./run.sh 1
    ```
    *O script configura as rotas com os IPs estáticos da rede, inicia o Docker Compose correspondente à máquina (`docker-compose.pcX.yml`) e abre janelas `xterm` com o acompanhamento dos logs de cada serviço local.*

---

## 🌐 Acesso ao Dashboard

Independentemente do método de execução, você poderá acompanhar o comportamento do sistema por duas interfaces:

*   **Web Dashboard:** Acesse via navegador em `http://localhost:8080` (ou `http://[IP_DA_MAQUINA]:8180` no cenário multi-máquinas). Exibe gráficos em tempo real, mapas com a posição cartesiana de cada drone, fila de prioridades consolidada e logs globais.
*   **Dashboard TUI:** Console interativo construído na linha de comando que exibe o monitoramento das mensagens trocadas diretamente no console de cada base.

---

## ⚙️ Configuração de Frequência de Requisições

Caso seja necessário aumentar ou diminuir a quantidade de requisições geradas no sistema (eventos `NORMAL` e `CRITICAL`) durante a avaliação, você pode alterar a porcentagem de probabilidade no código dos sensores.

Edite o arquivo **[`cmd/sensor/main.go`](cmd/sensor/main.go)** na função `generateEvent()`, alterando os valores de probabilidade:
```go
func generateEvent(n int64) models.Request {
	// ...
	roll := rng.Float32()

	// Ajuste as porcentagens abaixo para testar diferentes cargas:
	if roll > 0.800 { // Maior que 0.80 = 20% de chance
		reqType = models.TypeCritical
		// ...
	} else if roll > 0.100 { // Maior que 0.10 e Menor que 0.80 = 70% de chance
		reqType = models.TypeNormal
		// ...
	}
	// ...
}
```

---

## 💡 Cenários Práticos de Testes de Falhas

### Cenário 1: Queda de Base de Setor e Adoção Automática de Drones
1.  Inicie a simulação local com `run_local.ps1`.
2.  Verifique no Web Dashboard que o `drone-A1` está registrado na `base-A`.
3.  Feche a janela de terminal da **`base-A`** (ou finalize seu processo).
4.  Aguarde 9 segundos (tempo limite para detecção de inatividade UDP pelas bases vizinhas).
5.  A base sobrevivente (ex: `base-B`) detectará a queda, enviará um comando de herança (`MsgReassign`) para o `drone-A1` e assumirá o controle sobre ele. O `drone-A1` passará a constar sob a gerência da base sobrevivente no painel Web.

### Cenário 2: Ocorrências Concorrentes e Exclusão Mútua
1.  Verifique a estabilidade da fila e certifique-se de que os drones estão no status `IDLE`.
2.  Dispare manualmente dois alertas de altíssima prioridade simultaneamente nos sensores em setores distintos.
3.  Ao receberem as ocorrências, ambas as bases competirão por locks de despacho.
4.  O algoritmo de Ricart-Agrawala ordenará os lock requests via Relógio Lógico de Lamport e concederá a permissão apenas à base com menor carimbo temporal (ou desempate determinado), garantindo que apenas um drone atenda cada ocorrência por vez.