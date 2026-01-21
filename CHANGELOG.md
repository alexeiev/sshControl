# Changelog

Todas as mudanças notáveis neste projeto serão documentadas neste arquivo.

O formato é baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.0.0/),
e este projeto adere ao [Semantic Versioning](https://semver.org/lang/pt-BR/).

## [Unreleased]

### Added
- Suporte a tunnel SSH reverso para compartilhar proxy HTTP/HTTPS/FTP (`-p, --proxy`)
- Configuração de proxy no `config.yaml` (campos `proxy` e `proxy_port`)
- Status do proxy exibido no banner da TUI
- Documentação completa sobre uso de proxy no README.md

## [0.2.0] - 2026-01-21

### Added
- Funcionalidade de proxy reverso via SSH para compartilhar proxy da máquina local com hosts remotos

### Changed
- Melhorias na documentação

## [0.1.4] - 2026-01-21

### Fixed
- Correção na execução direta de conexão SSH
- Modo silencioso adicionado no curl do script de instalação

## [0.1.3] - 2026-01-20

### Fixed
- Correção na apresentação da versão na TUI

## [0.1.2] - 2026-01-20

### Added
- Verificação automática de atualizações ao executar o comando

### Fixed
- Melhorias no texto de ajuda do comando update

## [0.1.1] - 2026-01-20

### Fixed
- Correção no script de instalação
- Correção no bug de captura de output no script de instalação
- Atualização das versões de imagem do macOS no GitHub Actions

## [0.1.0] - 2026-01-20

### Added
- GitHub Pages para documentação
- Licença GPL-3.0 ao projeto

### Changed
- Modificada a forma de download dos binários

## [0.0.9] - 2026-01-20

### Added
- Primeira versão pública estável
- Modo interativo (TUI) com menu visual
- Modo direto de conexão via CLI
- Suporte completo a Jump Hosts
- Execução de comandos em múltiplos hosts simultaneamente
- Autenticação flexível (chaves SSH, SSH Agent, senha)
- Sistema de auto-atualização

### Fixed
- Correção de módulos e adição de informação de versão
- Correção do bug que pedia senha múltiplas vezes para usuários sem chave SSH
- Correção no tratamento de Jump Hosts com múltiplas máquinas e usuários diferentes

[Unreleased]: https://github.com/alexeiev/sshControl/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/alexeiev/sshControl/compare/v0.1.4...v0.2.0
[0.1.4]: https://github.com/alexeiev/sshControl/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/alexeiev/sshControl/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/alexeiev/sshControl/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/alexeiev/sshControl/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/alexeiev/sshControl/compare/v0.0.9...v0.1.0
[0.0.9]: https://github.com/alexeiev/sshControl/releases/tag/v0.0.9
