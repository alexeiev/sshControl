# GitHub Pages - sshControl

Este diretório contém os arquivos para o GitHub Pages do projeto sshControl.

## Estrutura

- **`index.html`** - Página principal exibida em https://sshcontrol.alexeiev.me
- **`style.css`** - Estilos da página
- **`install`** - Script de instalação bash (sem extensão)
- **`404.html`** - Página de erro customizada
- **`CNAME`** - Arquivo de configuração do domínio customizado

## URLs

- **Página Web**: https://sshcontrol.alexeiev.me
- **Script de Instalação**: https://sshcontrol.alexeiev.me/install

## Comando de Instalação

```bash
curl -fsSL https://sshcontrol.alexeiev.me/install | bash
```

## Atualização

Para atualizar o conteúdo:

1. Edite os arquivos neste diretório
2. Commit e push para a branch `main`
3. GitHub Pages atualiza automaticamente em 1-2 minutos

### Atualizar Página Web

```bash
# Edite o HTML e CSS
vim docs/index.html
vim docs/style.css

# Commit
git add docs/
git commit -m "docs: Atualiza página web"
git push origin main
```

### Atualizar Script de Instalação

```bash
# 1. Edite o script original
vim install.sh

# 2. Copie para docs/install (importante: sem extensão .sh)
cp install.sh docs/install

# 3. Commit ambos
git add install.sh docs/install
git commit -m "feat: Atualiza script de instalação"
git push origin main
```

## Configuração

Veja `GITHUB_PAGES_SETUP.md` na raiz do repositório para instruções completas de como configurar o GitHub Pages e o DNS customizado.

## Manutenção

- O GitHub Pages é servido da pasta `/docs` na branch `main`
- HTTPS é gerenciado automaticamente pelo GitHub via Let's Encrypt
- O domínio customizado é configurado via arquivo `CNAME`
- Cache do navegador é configurado automaticamente pelo GitHub

## Verificação

Para testar localmente, você pode usar um servidor HTTP simples:

```bash
# Python 3
cd docs
python3 -m http.server 8000

# Abra no browser: http://localhost:8000
```

## Recursos

- [GitHub Pages Documentation](https://docs.github.com/en/pages)
- [Managing Custom Domains](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site)
