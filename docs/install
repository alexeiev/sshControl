#!/usr/bin/env bash
#
# Script de instalação do sshControl (sc)
# Este script baixa e instala automaticamente a versão mais recente
#
# Uso:
#   curl -fsSL https://raw.githubusercontent.com/alexeiev/sshControl/main/install.sh | bash
#   ou
#   wget -qO- https://raw.githubusercontent.com/alexeiev/sshControl/main/install.sh | bash
#
# Instalação customizada:
#   curl -fsSL https://raw.githubusercontent.com/alexeiev/sshControl/main/install.sh | bash -s -- --dir=/custom/path
#

set -e

# Configurações
REPO_OWNER="alexeiev"
REPO_NAME="sshControl"
BINARY_NAME="sc"
INSTALL_DIR="/usr/local/bin"
GITHUB_API="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"

# Cores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Funções auxiliares
print_info() {
    echo -e "${BLUE}ℹ️  ${1}${NC}" >&2
}

print_success() {
    echo -e "${GREEN}✅ ${1}${NC}" >&2
}

print_warning() {
    echo -e "${YELLOW}⚠️  ${1}${NC}" >&2
}

print_error() {
    echo -e "${RED}❌ ${1}${NC}" >&2
}

# Detecta sistema operacional
detect_os() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$OS" in
        darwin*)
            echo "darwin"
            ;;
        linux*)
            echo "linux"
            ;;
        *)
            print_error "Sistema operacional não suportado: $OS"
            exit 1
            ;;
    esac
}

# Detecta arquitetura
detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)
            echo "amd64"
            ;;
        arm64|aarch64)
            echo "arm64"
            ;;
        *)
            print_error "Arquitetura não suportada: $ARCH"
            exit 1
            ;;
    esac
}

# Verifica dependências
check_dependencies() {
    local deps=("curl" "tar")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            print_error "Dependência não encontrada: $dep"
            print_info "Por favor, instale $dep e tente novamente"
            exit 1
        fi
    done
}

# Obtém a última versão do GitHub
get_latest_version() {
    print_info "Consultando última versão..."

    # Tenta obter via API do GitHub
    VERSION=$(curl -fsSL "$GITHUB_API" | grep '"tag_name"' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')

    if [ -z "$VERSION" ]; then
        print_error "Não foi possível obter a versão mais recente"
        exit 1
    fi

    print_success "Versão mais recente: $VERSION"
    echo "$VERSION"
}

# Baixa e instala o binário
install_binary() {
    local version=$1
    local os=$2
    local arch=$3
    local install_dir=$4

    # Monta o nome do arquivo
    local filename="${BINARY_NAME}-${version}-${os}-${arch}.tar.gz"
    local download_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${version}/${filename}"

    print_info "Baixando ${filename}..."

    # Cria diretório temporário
    local tmp_dir=$(mktemp -d)
    trap "rm -rf $tmp_dir" EXIT

    # Baixa o arquivo
    if ! curl -fL "$download_url" -o "$tmp_dir/$filename"; then
        print_error "Falha ao baixar o binário"
        exit 1
    fi

    print_success "Download concluído"

    # Extrai o binário
    print_info "Extraindo binário..."
    if ! tar -xzf "$tmp_dir/$filename" -C "$tmp_dir"; then
        print_error "Falha ao extrair o arquivo"
        exit 1
    fi

    # Verifica se o diretório de instalação existe
    if [ ! -d "$install_dir" ]; then
        print_warning "Diretório $install_dir não existe"
        print_info "Criando diretório..."
        if ! sudo mkdir -p "$install_dir"; then
            print_error "Falha ao criar diretório de instalação"
            exit 1
        fi
    fi

    # Move o binário para o diretório de instalação
    print_info "Instalando em $install_dir/$BINARY_NAME..."

    # Verifica se precisa de sudo
    if [ -w "$install_dir" ]; then
        mv "$tmp_dir/$BINARY_NAME" "$install_dir/$BINARY_NAME"
        chmod +x "$install_dir/$BINARY_NAME"
    else
        sudo mv "$tmp_dir/$BINARY_NAME" "$install_dir/$BINARY_NAME"
        sudo chmod +x "$install_dir/$BINARY_NAME"
    fi

    # Remove atributo de quarentena no macOS
    if [ "$os" = "darwin" ]; then
        print_info "Removendo atributo de quarentena do macOS..."
        if [ -w "$install_dir/$BINARY_NAME" ]; then
            xattr -d com.apple.quarantine "$install_dir/$BINARY_NAME" 2>/dev/null || true
        else
            sudo xattr -d com.apple.quarantine "$install_dir/$BINARY_NAME" 2>/dev/null || true
        fi
    fi

    print_success "Instalação concluída!"
}

# Verifica a instalação
verify_installation() {
    local install_dir=$1

    if command -v "$BINARY_NAME" &> /dev/null; then
        local installed_version=$("$BINARY_NAME" --version 2>/dev/null | head -n1 || echo "desconhecida")
        print_success "$BINARY_NAME instalado com sucesso!"
        print_info "Localização: $(which $BINARY_NAME)"
        print_info "Versão: $installed_version"
        return 0
    elif [ -f "$install_dir/$BINARY_NAME" ]; then
        print_warning "$BINARY_NAME instalado em $install_dir/$BINARY_NAME, mas não está no PATH"
        print_info "Adicione $install_dir ao seu PATH ou mova o binário para um diretório já no PATH"
        return 1
    else
        print_error "Instalação falhou"
        return 1
    fi
}

# Função principal
main() {
    # Parse argumentos
    while [[ $# -gt 0 ]]; do
        case $1 in
            --dir=*)
                INSTALL_DIR="${1#*=}"
                shift
                ;;
            --dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            -h|--help)
                echo "Uso: install.sh [opções]"
                echo ""
                echo "Opções:"
                echo "  --dir=DIR    Diretório de instalação (padrão: /usr/local/bin)"
                echo "  -h, --help   Exibe esta mensagem"
                exit 0
                ;;
            *)
                print_error "Opção desconhecida: $1"
                exit 1
                ;;
        esac
    done

    echo "" >&2
    print_info "======================================"
    print_info "   Instalador do sshControl (sc)"
    print_info "======================================"
    echo "" >&2

    # Verifica dependências
    check_dependencies

    # Detecta sistema
    OS=$(detect_os)
    ARCH=$(detect_arch)
    print_info "Sistema detectado: $OS ($ARCH)"

    # Obtém versão
    VERSION=$(get_latest_version)

    # Instala
    install_binary "$VERSION" "$OS" "$ARCH" "$INSTALL_DIR"

    echo "" >&2
    print_info "======================================"

    # Verifica instalação
    if verify_installation "$INSTALL_DIR"; then
        echo "" >&2
        print_info "Próximos passos:"
        print_info "  1. Execute 'sc --help' para ver os comandos disponíveis"
        print_info "  2. Configure seus hosts em ~/.sshControl/config.yaml"
        print_info "  3. Execute 'sc' para iniciar o modo interativo"
        echo "" >&2
        print_success "Instalação bem-sucedida! 🎉"
    else
        exit 1
    fi

    print_info "======================================"
    echo "" >&2
}

# Executa
main "$@"
