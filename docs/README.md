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
2. Abra um Pull Request para `main`; o workflow `GitHub Pages` prepara o artefato do site para validação, sem publicar
3. Após o merge, o mesmo workflow publica o conteúdo de `docs` automaticamente

O workflow está em [`.github/workflows/pages.yml`](../.github/workflows/pages.yml). Ele publica os arquivos estáticos diretamente, sem Jekyll, usando `ubuntu-24.04` e actions com Node.js 24. Também pode ser executado manualmente pela aba Actions; a publicação só ocorre na branch `main`.

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

Veja [`docs_int/GITHUB_PAGES_SETUP.md`](../docs_int/GITHUB_PAGES_SETUP.md) para instruções completas de como configurar o GitHub Pages e o DNS customizado.

Em **Settings → Pages → Build and deployment → Source**, selecione **GitHub Actions**. O domínio personalizado deve continuar configurado como `sshcontrol.alexeiev.me` nas configurações do Pages; o arquivo `CNAME` permanece no diretório `docs` como referência.

Para voltar à publicação automática anterior, altere a fonte para **Deploy from a branch**, com branch `main` e pasta `/docs`, e desative o workflow `GitHub Pages` na aba Actions.

## Identidade Visual

As cores e fontes ficam como variáveis no `:root` de `style.css`:

- **Fundo**: lavanda claro (`--ground: #f2f1f7`), tema único
- **Marca**: violeta (`--brand`), o mesmo tom do menu TUI (`#7D56F4` em `cmd/menu.go`)
- **Sinal**: âmbar, usado só em estados ativos (prompt `$`, cursor, rota do tunnel, selos de versão)
- **Terminais e caixas de comando**: sempre escuros (`--term-*`)
- **Fontes** (Google Fonts): Martian Mono (títulos), Hanken Grotesk (texto), JetBrains Mono (código)

As saídas mostradas nos terminais da página seguem o formato real impresso pelo `sc`. Ao mudar uma mensagem no código, atualize o exemplo correspondente no `index.html`.

## Manutenção

- O workflow `GitHub Pages` publica os arquivos de `/docs` da branch `main`
- HTTPS é gerenciado automaticamente pelo GitHub via Let's Encrypt
- O domínio customizado é mantido nas configurações do Pages; o arquivo `CNAME` documenta o domínio usado
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
