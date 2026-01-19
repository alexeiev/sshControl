# FEATURE
[V] Permitir o envio de comandos remotos a uma lista de hosts
[V] Criar uma opção para listar os hosts cadastrados
[V] Criar lista de JumpServers com itens ["Name", "User", "IP/Host", "Port" ]
[V] permitir uso de user diferente na maquina de salto e no alvo
[ ] Permitir o envio de arquivos a servidores remoto. (usar o tui para procurar o arquivo ou diretório)
[ ] Criar uma opção de tunnel (usar proxy via ssh)



# FIX
[V] permitir conexão por senha, sem a chave cadastrada no config.yaml
[V] Corrigir a execução de comandos usando multiplos hosts com Jump Host. O pedido de Senha para o Jump Host deve ser pedido uma vez apenas. Problema só acontece se o usuário não tiver chave ssh
 