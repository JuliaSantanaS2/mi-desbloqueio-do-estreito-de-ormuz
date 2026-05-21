# Sistema Distribuído de Drones — Estreito de Ormuz (PBL 2)

Sistema distribuído em Go com Docker para coordenar uma frota de drones autônomos de monitoramento marítimo. 
A arquitetura empregada é **Peer-to-Peer (P2P) Descentralizada**, garantindo ausência de ponto único de falha. A comunicação ocorre exclusivamente via protocolos TCP (dados confiáveis) e UDP (heartbeats).

---

## 🏗️ Arquitetura da Solução e Papéis
- **Bases de Setor (Nós P2P):** Formam o núcleo do sistema. Mantêm a fila distribuída de requisições, gerenciam a exclusão mútua e despacham drones. Se uma base cai, os drones são herdados automaticamente por outra.
- **Brokers:** Recebem eventos dos sensores e fazem o roteamento seguro para as bases.
- **Sensores:** Entidades na borda que geram ocorrências.
- **Drones:** Trabalhadores distribuídos. Respondem às bases e executam as missões baseadas em coordenadas (X, Y).
- **Dashboard:** Interface Web que exibe o estado consolidado reconstruindo os Relógios de Lamport.

## 🛡️ Tratamento de Falhas e Protocolo
- **Timeouts e ACKs (TCP):** As conexões utilizam `SetReadDeadline` e `SetWriteDeadline`. Além do controle da camada TCP, a aplicação exige confirmações explícitas. Ex: Quando a Base envia um comando `MsgDispatch`, ela aguarda ativamente um `ACK` do drone; se não receber, declara o drone offline imediatamente.
- **Detecção Rápida (UDP):** O sistema utiliza *Heartbeats UDP* para detecção de vitalidade. Ausência de pulso por 9s aciona o plano de contingência e redistribuição.

## 🔒 Concorrência Distribuída (Ricart-Agrawala + Lamport)
Para garantir que dois drones não sejam enviados para a mesma ocorrência (Não-duplicidade) e proteger recursos, o sistema utiliza o **Relógio Lógico de Lamport** atrelado a uma adaptação do **Algoritmo de Exclusão Mútua de Ricart-Agrawala**.
1. Uma base só atende uma requisição se solicitar um "Lock" (voto de permissão) a **todas as outras bases ativas**.
2. Apenas mediante a confirmação (`MsgLockReply.Granted = true`) de 100% da rede ativa o drone é despachado.
3. Requisições concorrentes são ordenadas na fila distribuída respeitando primeiro o grau de criticidade e, em caso de empate, o carimbo de tempo do Relógio de Lamport (Ordering).

---

## Estrutura do Projeto