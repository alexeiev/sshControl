# sshControl (sc)

[![Latest Release](https://img.shields.io/github/v/release/alexeiev/sshControl?label=version&color=blue)](https://github.com/alexeiev/sshControl/releases/latest)
[![License](https://img.shields.io/github/license/alexeiev/sshControl?color=green)](https://github.com/alexeiev/sshControl/blob/main/LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/alexeiev/sshControl)](https://go.dev/)
[![Build Status](https://img.shields.io/github/actions/workflow/status/alexeiev/sshControl/release.yml?branch%3Amain)](https://github.com/alexeiev/sshControl/actions)
[![Downloads](https://img.shields.io/github/downloads/alexeiev/sshControl/total?color=orange)](https://github.com/alexeiev/sshControl/releases)

Gerenciador de conexões SSH escrito em Go com interface interativa (TUI) e modo CLI direto.

## Características

- 🚀 **Modo Interativo (TUI)**: Menu visual para seleção de hosts
- ⚡ **Modo Direto**: Conecte rapidamente via linha de comando
- 🔗 **Jump Hosts**: Suporte completo para conexões via bastion/jump hosts
- 🏷️ **Tags para Hosts**: Agrupe hosts por tags e execute comandos em lote por grupo
- 🌐 **Proxy Reverso**: Compartilhe proxy HTTP/HTTPS/FTP da máquina local com hosts remotos
- 📦 **Execução em Lote**: Execute comandos em múltiplos hosts simultaneamente, usando hosts diretos, tags ou arquivos texto
- 🔐 **Autenticação Flexível**: Suporte para chaves SSH, SSH Agent e senha
- 🔑 **Auto-Instalação de Chaves**: Instala automaticamente sua chave pública no servidor após primeira conexão
- 🔒 **Controle de Senha**: Flag `-a` para solicitar senha antecipadamente (ideal para automações)
- 📝 **Auto-Criação de Hosts**: Salva automaticamente hosts não cadastrados no config.yaml
- 📁 **Cópia de Arquivos**: Transferência de arquivos via SFTP com suporte a múltiplos hosts
- 🚇 **Port Forward**: Encaminhe portas locais para remotas via túnel SSH (similar ao kubectl port-forward)
- 🔍 **Modo Debug**: Flag `-v` para exibir informações detalhadas da conexão e facilitar diagnósticos
- 🔄 **Auto-Atualização**: Atualize para a versão mais recente com um comando

## Instalação

### Instalação Automática (Recomendado)

O script de instalação detecta automaticamente seu sistema operacional e arquitetura, baixa a versão correta e instala o binário:

```bash
curl -fsSL https://sshcontrol.alexeiev.me/install | bash
```

Ou usando a URL alternativa:
```bash
curl -fsSL https://raw.githubusercontent.com/alexeiev/sshControl/main/install.sh | bash
```

**Instalação customizada**:
```bash
# Instalar em diretório específico
curl -fsSL https://sshcontrol.alexeiev.me/install | bash -s -- --dir=$HOME/.local/bin

# Ver opções disponíveis
curl -fsSL https://sshcontrol.alexeiev.me/install | bash -s -- --help
```

O script automaticamente:
- Detecta seu OS (Linux/macOS) e arquitetura (amd64/arm64)
- Baixa a versão mais recente do GitHub
- Instala em `/usr/local/bin` (ou diretório especificado)
- Remove o atributo de quarentena no macOS (evita aviso de segurança)
- Verifica se a instalação foi bem-sucedida


### Compilar do Código Fonte

```bash
git clone https://github.com/alexeiev/sshControl.git
cd sshControl
make build
# Binários estarão em bin/amd64/sc e bin/arm64/sc
```

## Configuração

Na primeira execução, o sshControl cria automaticamente o arquivo de configuração em `~/.sshControl/config.yaml`.

### Exemplo de Configuração

```yaml
config:
  default_user: ubuntu
  auto_create: false          # Se true, salva hosts não cadastrados automaticamente
  dir_cp_default: ~/sshControl  # Diretório padrão para downloads via 'sc cp down'
  proxy: "192.168.0.1:3128"  # IP:PORT do proxy HTTP/HTTPS/FTP na máquina local
  proxy_port: 9999            # Porta local no host remoto para acessar o proxy
  users:
    - name: ubuntu
      ssh_keys:
        - ~/.ssh/id_rsa
        - ~/.ssh/id_ed25519
    - name: admin
      ssh_keys:
        - ~/.ssh/admin_key
  jump_hosts:
    - name: production-jump
      host: jump.production.example.com
      user: ubuntu
      port: 22
    - name: staging-jump
      host: jump.staging.example.com
      user: ubuntu
      port: 22

hosts:
  - name: webserver
    host: 192.168.1.50
    port: 22
    tags: 
      - web
      - production
  - name: database
    host: 192.168.1.51
    port: 22
    tags: 
      - db
      - production
  - name: app-server
    host: 10.0.1.100
    port: 22
    tags: 
      - app
      -  production
  - name: staging-web
    host: 10.0.2.50
    port: 22
    tags: 
      - web
      - staging
```

## Uso

### Modo Interativo (TUI)

```bash
# Abre menu interativo
sc

# Menu com usuário específico (config.users[])
sc -u admin

# Menu com Jump Host
sc -j production-jump

# Menu com Proxy via SSH Reverso
sc -p
```

### Conexão Direta

```bash
# Conecta a host configurado
sc webserver

# Conecta a IP diretamente
sc 192.168.1.50

# Especifica usuário e porta
sc ubuntu@192.168.1.50:2222

# Via jump host (por nome)
sc -j production-jump webserver

# Via jump host (por índice)
sc -j 1 webserver

# Com proxy reverso habilitado
sc -p webserver

# Com jump host e proxy
sc -j production-jump -p webserver
```

### Execução de Comandos

**Host único**:
```bash
# Em host configurado
sc -c "uptime" webserver

# Em IP direto
sc -c "df -h" 192.168.1.50

# Com jump host
sc -j production-jump -c "systemctl status nginx" app-server
```

**Múltiplos hosts**:
```bash
# Em vários hosts configurados
sc -c "uptime" -l web1 web2 web3

# Mistura de hosts e IPs
sc -c "free -h" -l webserver 192.168.1.50 ubuntu@192.168.1.51

# Lendo hosts de um arquivo texto
sc -c "uptime" -l lista.txt

# Combina arquivo, tags e hosts na mesma execução
sc -c "hostname" -l lista.txt @web 10.10.10.10

# Via jump host
sc -j 1 -c "df -h" -l db1 db2 db3

# Solicitando senha antecipadamente (útil para automações)
sc -a -c "hostname" -l web1 web2 web3
```

O arquivo informado em `-l` pode conter hosts separados por vírgula, ponto e vírgula ou um por linha. Também é possível misturar aliases do `config.yaml`, IPs, conexões diretas como `user@host:porta` e tags (`@tag`) no mesmo arquivo.

**Usando Tags** (prefixo `@`):
```bash
# Executar em todos os hosts com tag "web"
sc -c "uptime" -l @web

# Executar em múltiplas tags
sc -c "df -h" -l @web @db

# Combinar tags com hosts específicos
sc -c "hostname" -l @production server1 192.168.1.100

# Com jump host
sc -j 1 -c "systemctl status nginx" -l @web
```

**Controle de Autenticação**:
```bash
# Sem -a: tenta chave SSH, falha silenciosamente (ideal para automações/loops)
for host in web1 web2 web3; do
    sc -c "uptime" $host
done

# Com -a: solicita senha uma vez antes de executar (quando chaves não estão instaladas)
sc -a -c "uptime" -l web1 web2 web3
```

### Cópia de Arquivos (SFTP)

**Download de arquivos do servidor remoto**:
```bash
# Baixa arquivo para diretório padrão (dir_cp_default do config)
sc cp down webserver /var/log/app.log

# Baixa arquivo para diretório específico
sc cp down webserver /var/log/app.log ./logs/

# Baixa diretório recursivamente
sc cp down -r webserver /etc/nginx/ ./nginx-backup/

# Com jump host
sc cp down -j 1 db-prod /backup/dump.sql ./

# Usando ~ para home do usuário remoto
sc cp down webserver ~/app/config.yaml ./
```

**Upload de arquivos para servidor(es)**:
```bash
# Envia para o home do usuário remoto (~)
sc cp up ./config.yaml webserver

# Envia para diretório específico
sc cp up ./config.yaml /etc/app/ webserver

# Envia para múltiplos hosts em paralelo
sc cp up -l web1 web2 web3 ./script.sh /opt/scripts/ 

# Envia usando arquivo texto com a lista de hosts
sc cp up -l lista.txt ./script.sh /opt/scripts/

# Combina arquivo, tags e hosts na mesma chamada
sc cp up -l lista.txt @web app1 ./deploy.sh /opt/app/

# Envia diretório recursivamente
sc cp up -r ./dist/ /var/www/html/ webserver

# Com jump host e múltiplos hosts
sc cp up -l app1 app2 -j prod-jump ./app.jar /opt/app/

# Usando tags
sc cp up -l @web ./deploy.sh /opt/
```

**Flags do comando cp**:
- `-r, --recursive`: Copia diretórios recursivamente
- `-l, --list`: Envia para múltiplos hosts (apenas `up`)
- `-j, --jump <jump>`: Usa jump host
- `-u, --user <user>`: Usa usuário específico
- `-a, --ask-password`: Solicita senha antes
- `-v, --verbose`: Modo debug (informações detalhadas da conexão)

### Modo Debug (Verbose)

Use a flag `-v` para exibir informações detalhadas da conexão, útil para diagnosticar problemas de autenticação, rede ou configuração:

```bash
# Conexão direta com debug
sc -v webserver

# Comando remoto com debug
sc -v -c "uptime" webserver

# Com jump host
sc -v -j 1 webserver

# Múltiplos hosts
sc -v -c "uptime" -l @web

# SFTP com debug
sc cp down -v webserver /var/log/app.log ./

# Port forward com debug
sc port-forward -v webserver 8080:80
```

**Exemplo de saída**:
```
[DEBUG] Usuário: ubuntu
[DEBUG] Host: 192.168.1.50:22
[DEBUG] Chave SSH: ~/.ssh/id_rsa ... OK
[DEBUG] Chave SSH: ~/.ssh/id_ed25519 ... falha ao ler arquivo
[DEBUG] SSH Agent: disponível
[DEBUG] Métodos de autenticação: [publickey (1 chave(s)), agent, password (interativa)]
[DEBUG] Conectando diretamente a 192.168.1.50:22...
[DEBUG] Conexão direta estabelecida
[DEBUG] Conexão SSH estabelecida com sucesso
[DEBUG] Solicitando PTY (xterm-256color, 120x40)
[DEBUG] Iniciando sessão interativa...
```

### Comandos Úteis

```bash
# Listar servidores e jump hosts cadastrados
sc -s

# Listar servidores filtrados por tag
sc -s @ansible
sc -s @production

# Verificar versão
sc -V
sc --version

# Atualizar para versão mais recente
sc update
# Ou com sudo se instalado em /usr/local/bin
sudo sc update

# Port forward (túnel SSH)
sc port-forward webserver 8080:80

# Manual completo com exemplos detalhados
sc man

# Ajuda rápida
sc --help
```

## Características Detalhadas

### Auto-Instalação de Chaves SSH

O sshControl automatiza a instalação de chaves públicas SSH nos servidores remotos, eliminando a necessidade de usar `ssh-copy-id` manualmente.

**Como Funciona**:

1. **Validação**: Na inicialização, verifica se os arquivos `.pub` existem para cada chave privada configurada
2. **Primeira Conexão**: Ao conectar com senha (quando chave ainda não está instalada), automaticamente:
   - Lê o arquivo `.pub` correspondente à chave privada
   - Verifica se a chave já existe no `~/.ssh/authorized_keys` do servidor
   - Se não existir, adiciona a chave com permissões corretas
3. **Próximas Conexões**: Autentica automaticamente via chave SSH (sem senha)

**Exemplo Prático**:

```bash
# Primeira vez conectando ao servidor (sem chave instalada)
sc -a webserver
# Password for ubuntu@webserver: ********
# ✅ Chave pública instalada com sucesso no servidor remoto

# Próximas conexões já usam a chave (sem senha)
sc webserver
# 🔗 Conectando...
#    ubuntu@192.168.1.50 (key: ~/.ssh/id_rsa)
```

**Avisos**:

Se o arquivo `.pub` não existir, você verá um aviso:
```
⚠️  Aviso: Chave pública não encontrada para usuário 'ubuntu': ~/.ssh/id_rsa.pub (auto-instalação desabilitada)
```

**Importante**:
- Funciona em **modo interativo**, **modo direto** e **múltiplos hosts**
- Requer autenticação bem-sucedida primeiro (senha, agent, etc.)
- Não sobrescreve chaves existentes, apenas adiciona
- Define permissões corretas automaticamente (700 para `.ssh`, 600 para `authorized_keys`)

### Tags para Hosts

Organize seus hosts em grupos usando tags para facilitar a execução de comandos em lote.

**Configuração**:

```yaml
hosts:
  - name: web1
    host: 192.168.1.10
    port: 22
    tags: 
      - web
      - production
      - nginx
  - name: web2
    host: 192.168.1.11
    port: 22
    tags: 
      - web
      - production
      - nginx
  - name: db-master
    host: 192.168.1.20
    port: 22
    tags: 
      - db
      - production
      - mysql
  - name: db-replica
    host: 192.168.1.21
    port: 22
    tags: 
      - db
      - production
      - mysql
  - name: staging-web
    host: 10.0.1.10
    port: 22
    tags: 
      - web
      - staging
```

**Uso com Tags**:

```bash
# Executar em todos os hosts com tag "web"
sc -c "nginx -t" -l @web

# Executar em múltiplas tags (união de hosts)
sc -c "df -h" -l @web @db

# Combinar tags com hosts específicos
sc -c "uptime" -l @production monitoring-server

# Apenas hosts de produção
sc -c "systemctl status nginx" -l @production

# Reiniciar MySQL em todos os servidores de banco
sc -c "systemctl restart mysql" -l @mysql
```

**Filtro na TUI**:

No modo interativo, pressione `/` e digite o nome de uma tag para filtrar os hosts:

```
Filtrar hosts...> production
```

Mostrará apenas hosts que possuem a tag "production".

**Listagem e Filtro por Tags**:

O comando `sc -s` exibe as tags de cada host. Use `sc -s @tag` para filtrar:

```bash
# Lista todos os servidores
sc -s

# Lista apenas servidores com tag "web"
sc -s @web

# Lista apenas servidores com tag "production"
sc -s @production
```

Exemplo de saída:
```
📋 Servidores com tag 'web':
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Nome                 Host:Porta                Tags
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
web1                 192.168.1.10:22           web, production, nginx
web2                 192.168.1.11:22           web, production, nginx
```

**Casos de Uso**:

1. **Ambientes**: Separe hosts por ambiente (`production`, `staging`, `development`)
2. **Serviços**: Agrupe por tipo de serviço (`web`, `db`, `cache`, `queue`)
3. **Aplicações**: Identifique a aplicação (`nginx`, `mysql`, `redis`)
4. **Regiões**: Organize por localização (`us-east`, `eu-west`, `sa-east`)

### Auto-Criação de Hosts

O sshControl pode salvar automaticamente hosts não cadastrados no arquivo de configuração. Isso é útil para manter um registro de todos os servidores que você acessa.

**Configuração**:

```yaml
config:
  auto_create: true  # Habilita auto-criação de hosts
```

**Como Funciona**:

1. Quando você conecta a um host não cadastrado (por IP ou hostname direto)
2. Se `auto_create: true`, o host é salvo automaticamente após conexão bem-sucedida
3. O host recebe a tag `autocreated` para identificação
4. Uma mensagem é exibida solicitando que você finalize a configuração

**Exemplo**:

```bash
# Conecta a um host não cadastrado
sc 192.168.1.100

# Após a sessão SSH, se auto_create estiver habilitado:
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# ✅ Host adicionado automaticamente ao config.yaml:
#    name: 192.168.1.100
#    host: 192.168.1.100
#    port: 22
#    tags: [autocreated]
#
# 📝 Finalize a configuração do host editando o arquivo:
#    ~/.sshControl/config.yaml
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**Comportamento da Tag `autocreated`**:

- Hosts com esta tag **NÃO aparecem** no menu interativo (TUI)
- Hosts **aparecem normalmente** na listagem `sc -s`
- Você pode executar comandos usando `@autocreated`:
  ```bash
  sc -c "uptime" -l @autocreated
  ```
- Após configurar o host (adicionar nome amigável, outras tags), remova a tag `autocreated`

**Múltiplos Hosts**:

A auto-criação também funciona em modo múltiplos hosts:

```bash
sc -c "hostname" -l 192.168.1.10 192.168.1.11 192.168.1.12

# Após execução bem-sucedida, todos os hosts novos são salvos
```

### Jump Hosts

Configure múltiplos jump hosts e use-os por nome ou índice:

```yaml
config:
  jump_hosts:
    - name: production-jump  # índice 1
      host: bastion1.prod.com
      user: ubuntu
      port: 22
    - name: staging-jump     # índice 2
      host: bastion.staging.com
      user: ubuntu
      port: 22
```

```bash
# Por nome
sc -j production-jump webserver

# Por índice
sc -j 1 webserver
```

### Proxy Reverso

O sshControl permite compartilhar um proxy HTTP/HTTPS/FTP da sua máquina local com hosts remotos através de um tunnel SSH reverso. Isso é útil quando hosts remotos não têm acesso direto à internet mas precisam acessar recursos externos.

**Configuração do Proxy**:

```yaml
config:
  proxy: "192.168.0.1:3128"  # Endereço do proxy na máquina local
  proxy_port: 9999            # Porta que será aberta no host remoto
```

**Como Usar**:

```bash
# Conectar com proxy habilitado
sc -p webserver

# Com jump host e proxy
sc -j production-jump -p app-server

# Modo interativo com proxy
sc -p
```

**No Host Remoto**:

Após conectar com `-p`, configure as variáveis de ambiente para usar o proxy:

```bash
export https_proxy=http://127.0.0.1:9999
export http_proxy=http://127.0.0.1:9999
export ftp_proxy=http://127.0.0.1:9999

# ou apenas
export {https,http,ftp}_proxy=http://127.0.0.1:9999

# Testar
curl -I http://google.com
```

**Importante**:
- O tunnel permanece ativo durante toda a sessão SSH
- Com jump host, o proxy é configurado apenas no host final (target), não no jump host
- O proxy deve estar acessível a partir da máquina onde você executa o `sc`

### Port Forward (Túnel SSH)

O sshControl permite encaminhar uma porta local para uma porta remota através de um túnel SSH, similar ao `kubectl port-forward` ou `ssh -L`.

**Sintaxe**:

```bash
sc port-forward [flags] <host> <local_port:remote_port>
```

**Exemplos**:

```bash
# Encaminha porta local 8080 para porta remota 80
sc port-forward webserver 8080:80

# Acessa MySQL remoto (3306) via porta local 3307
sc port-forward db-server 3307:3306

# Via jump host
sc port-forward -j production-jump db-prod 5433:5432

# Com usuário específico
sc port-forward -u deploy app-server 9000:8080

# Solicita senha antes de conectar
sc port-forward -a webserver 8080:80
```

**Características**:

- **Logs em tempo real**: Mostra cada conexão com origem, bytes transferidos e duração
- **Estatísticas da sessão**: Ao encerrar (Ctrl+C), exibe total de conexões e bytes
- **Suporte completo**: Jump hosts (`-j`), usuário específico (`-u`), senha (`-a`), debug (`-v`)

**Exemplo de saída**:

```
🚇 Port Forward Ativo
   Local:  127.0.0.1:8080
   Remoto: 127.0.0.1:80 (via webserver)

📋 Log de conexões:
────────────────────────────────────────────────────────────────
[14:32:15] #1 ✅ Conexão de 127.0.0.1:52341
[14:32:18] #1 🔚 Encerrada (↑1.2 KB ↓4.5 KB)
[14:33:01] #2 ✅ Conexão de 127.0.0.1:52342
...
```

**Casos de Uso**:

1. **Acesso a bancos de dados**: Conecte-se a MySQL/PostgreSQL remotos via localhost
2. **APIs internas**: Acesse APIs que só estão disponíveis na rede interna
3. **Dashboards**: Acesse interfaces web de monitoramento (Grafana, Kibana, etc.)
4. **Debug**: Conecte debuggers a aplicações remotas

### Autenticação

Ordem de tentativa de autenticação:
1. Chave SSH (especificada no config)
2. SSH Agent (se disponível)
3. Senha (solicitada interativamente ou com `-a`)

**Controle de Senha com Flag `-a`**:

A flag `-a` ou `--ask-password` permite controlar quando a senha é solicitada:

```bash
# Sem -a: senha solicitada interativamente como fallback (modo single host)
sc webserver

# Sem -a: em múltiplos hosts, tenta apenas chave SSH (ideal para automações)
sc -c "uptime" -l web1 web2 web3

# Com -a: solicita senha ANTES de tentar conectar
sc -a webserver
sc -a -c "uptime" -l web1 web2 web3
```

**Casos de Uso**:

1. **Automações/Scripts**: Use SEM `-a` para não interromper loops
   ```bash
   for host in web{1..10}; do
       sc -c "uptime" $host  # Falha silenciosamente se chave não funcionar
   done
   ```

2. **Primeira Conexão**: Use COM `-a` quando chaves ainda não estão instaladas
   ```bash
   # Solicita senha uma vez, instala chave, próximas conexões sem senha
   sc -a -c "hostname" -l server1 server2 server3
   ```

3. **Servidores Sem Chave**: Use COM `-a` quando precisa usar senha
   ```bash
   sc -a production-db
   ```

### Execução Paralela

O modo múltiplos hosts (`-l`) executa comandos simultaneamente:

```bash
sc -c "uptime" -l server1 server2 server3 server4
```

Exibe resultados organizados com:
- ✅ Sucesso ou ❌ Falha por host
- Exit code de cada comando
- Tempo total de execução
- Resumo com contadores

### Cópia de Arquivos (SFTP)

O sshControl permite transferir arquivos entre a máquina local e servidores remotos via SFTP.

**Configuração**:

```yaml
config:
  dir_cp_default: ~/sshControl  # Diretório padrão para downloads
```

**Download (`sc cp down`)**:

Baixa arquivos ou diretórios do servidor remoto para a máquina local.

```bash
# Sintaxe
sc cp down [flags] <host> <caminho_remoto> [destino_local]

# Se destino_local não for especificado, usa dir_cp_default do config
sc cp down webserver /var/log/app.log

# Baixar diretório recursivamente
sc cp down -r webserver /etc/nginx/ ./backup/
```

**Upload (`sc cp up`)**:

Envia arquivos ou diretórios para servidor(es) remoto(s).

```bash
# Sintaxe - Host único
sc cp up [flags] <arquivo_local> [destino_remoto] <host>

# Sintaxe - Múltiplos hosts (hosts vêm após -l)
sc cp up -l [flags] <hosts...> <arquivo_local> [destino_remoto]

# Se destino_remoto não for especificado, usa o home do usuário (~)
sc cp up ./config.yaml webserver

# Enviar para múltiplos hosts (hosts após -l)
sc cp up -l web1 web2 web3 ./script.sh /opt/

# Enviar diretório recursivamente
sc cp up -r ./dist/ /var/www/html/ webserver
```

**Características**:

- **Barra de progresso**: Exibe progresso em tempo real durante transferências
- **Múltiplos hosts**: Upload simultâneo para vários servidores com `-l`
- **Recursivo**: Copia diretórios completos com `-r`
- **Jump hosts**: Suporte total a conexões via bastion com `-j`
- **Expansão de `~`**: Detecta e corrige automaticamente a expansão do shell local

**Nota sobre `~`**:

Quando você usa `~` no caminho remoto, o shell local pode expandir para seu home local. O sshControl detecta isso automaticamente e converte para o home do usuário remoto:

```bash
# Mesmo que o shell expanda ~/logs para /Users/seu_usuario/logs,
# o sshControl converte para /home/ubuntu/logs no servidor
sc cp down webserver ~/logs/app.log ./
```

### Auto-Atualização

```bash
# Atualizar (pode precisar de sudo se instalado em /usr/local/bin)
sc update
# ou
sudo sc update
```

O comando:
1. Verifica a última versão no GitHub
2. Compara com a versão atual
3. Solicita confirmação do usuário
4. Baixa o binário apropriado para seu OS/arquitetura
5. Substitui o binário atual (com backup)
6. Confirma a atualização

**Nota**: Se o sshControl foi instalado em `/usr/local/bin`, você precisará usar `sudo sc update`. Se instalou em um diretório pessoal (como `~/.local/bin`), não precisa de sudo.

## Desenvolvimento

### Build Local

```bash
# Compila para Linux e macOS
make build

# Executa sem compilar
go run .

# Build com versão customizada
VERSION=v1.0.0 make build
```

### Criar uma Release

```bash
# 1. Commite todas as mudanças
git add .
git commit -m "Release v1.0.0"

# 2. Crie e envie a tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin main
git push origin v1.0.0
```

O GitHub Actions automaticamente:
- Compila para todas as plataformas
- Cria arquivos tar.gz
- Gera checksums
- Publica a release

## Requisitos

- Go 1.25+ (para compilar)
- Acesso SSH aos hosts desejados
- Git (para versionamento durante build)

## Licença

Este projeto é distribuído sob a licença GPL-3.0. Veja o arquivo [LICENSE](https://github.com/alexeiev/sshControl/blob/main/LICENSE) para mais detalhes.

## Contribuindo

Contribuições são bem-vindas! Por favor:
1. Fork o projeto
2. Crie uma branch para sua feature
3. Commit suas mudanças
4. Push para a branch
5. Abra um Pull Request

## Changelog

Veja o [CHANGELOG.md](CHANGELOG.md) para o histórico detalhado de mudanças em cada versão.

## Suporte

Para reportar bugs ou solicitar features, abra uma [issue](https://github.com/alexeiev/sshControl/issues).
