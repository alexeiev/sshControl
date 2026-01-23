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
- 🌐 **Proxy Reverso**: Compartilhe proxy HTTP/HTTPS/FTP da máquina local com hosts remotos
- 📦 **Execução em Lote**: Execute comandos em múltiplos hosts simultaneamente
- 🔐 **Autenticação Flexível**: Suporte para chaves SSH, SSH Agent e senha
- 🔑 **Auto-Instalação de Chaves**: Instala automaticamente sua chave pública no servidor após primeira conexão
- 🔒 **Controle de Senha**: Flag `-a` para solicitar senha antecipadamente (ideal para automações)
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
  - name: database
    host: 192.168.1.51
    port: 22
  - name: app-server
    host: 10.0.1.100
    port: 22
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

# Via jump host
sc -j 1 -c "df -h" -l db1 db2 db3

# Solicitando senha antecipadamente (útil para automações)
sc -a -c "hostname" -l web1 web2 web3
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

### Comandos Úteis

```bash
# Listar servidores e jump hosts cadastrados
sc -s

# Verificar versão
sc --version

# Atualizar para versão mais recente
sc update
# Ou com sudo se instalado em /usr/local/bin
sudo sc update

# Ajuda
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
