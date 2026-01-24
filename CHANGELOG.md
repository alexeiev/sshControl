# Changelog

Todas as mudanças notáveis neste projeto serão documentadas neste arquivo.

O formato é baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.0.0/),
e este projeto adere ao [Semantic Versioning](https://semver.org/lang/pt-BR/).


## [0.3.0] - 2026-01-23

### Added

- Sistema de **Tags para Hosts**: Agrupe hosts por tags para organização e execução em lote
- Campo `tags: []` no cadastro de hosts no `config.yaml`
- Sintaxe `@tagname` para executar comandos em todos os hosts de uma tag
- Suporte a múltiplas tags na mesma execução (`sc -c "cmd" -l @web @database`)
- Combinação de tags com hosts específicos (`sc -c "cmd" -l @web server1`)
- Filtro por tags na TUI (digite `/` e busque pelo nome da tag)
- Exibição de tags na listagem de servidores (`sc -s`)
- Funções `FindHostsByTag()` e `GetAllTags()` no pacote config
- **Auto-criação de Hosts**: Opção `config.auto_create` para salvar automaticamente hosts não cadastrados
- Comando `sc man` com manual completo e exemplos de uso detalhados
- Hosts criados automaticamente recebem a tag `autocreated`
- Hosts com tag `autocreated` não aparecem na TUI mas podem ser usados via CLI e `@autocreated`
- Mensagem informativa após conexão/execução bem-sucedida solicitando finalização da configuração

### Changed

- Formato de saída do comando `sc -s` agora inclui coluna de Tags
- TUI exibe tags dos hosts na descrição (entre colchetes)
- TUI filtra automaticamente hosts com tag `autocreated`
- Template de configuração padrão inclui exemplos de tags e opção `auto_create`
- Help (`sc --help`) simplificado com exemplos básicos e referência ao `sc man`

## [0.2.1.1] - 2026-01-22

### Added

- Auto-instalação de chaves SSH públicas nos servidores remotos após primeira conexão bem-sucedida
- Validação de existência de arquivos `.pub` para chaves privadas configuradas com avisos informativos
- Flag `-a, --ask-password` para solicitar senha antecipadamente (útil para automações e scripts)
- Mensagens de erro melhoradas sugerindo uso de `-a` quando autenticação falha

### Changed

- Modo múltiplos hosts agora NÃO solicita senha automaticamente (evita interrupção em loops/automações)
- Senha em múltiplos hosts só é solicitada se flag `-a` for especificada
- Campo `InteractivePasswordAllowed` adicionado à struct `SSHConnection` para controle de prompt interativo

### Fixed

- Corrigido problema de múltiplas solicitações de senha simultâneas em modo múltiplos hosts
- Corrigido prompts de senha sobrepostos quando executando comandos em paralelo sem chave SSH instalada
- Mensagens de erro agora são mais claras, indicando quando usar `-a` para fornecer senha

## [0.2.1] - 2026-01-22

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

[Unreleased]: https://github.com/alexeiev/sshControl/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/alexeiev/sshControl/compare/v0.2.1.1...v0.3.0
[0.2.1.1]: https://github.com/alexeiev/sshControl/compare/v0.2.1...v0.2.1.1
[0.2.1]: https://github.com/alexeiev/sshControl/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/alexeiev/sshControl/compare/v0.1.4...v0.2.0
[0.1.4]: https://github.com/alexeiev/sshControl/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/alexeiev/sshControl/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/alexeiev/sshControl/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/alexeiev/sshControl/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/alexeiev/sshControl/compare/v0.0.9...v0.1.0
[0.0.9]: https://github.com/alexeiev/sshControl/releases/tag/v0.0.9
