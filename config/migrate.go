package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MigrateConfig verifica se o config.yaml do usuário possui todas as chaves do template.
// Chaves faltantes são adicionadas com os valores padrão do template, sem remover dados existentes.
func MigrateConfig(configPath string) error {
	// Lê o arquivo do usuário
	userData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("erro ao ler config para migração: %w", err)
	}

	// Parse do config do usuário como yaml.Node (preserva comentários e ordem)
	var userDoc yaml.Node
	if err := yaml.Unmarshal(userData, &userDoc); err != nil {
		return fmt.Errorf("erro ao parsear config do usuário: %w", err)
	}

	// Parse do template como yaml.Node
	var templateDoc yaml.Node
	if err := yaml.Unmarshal([]byte(defaultConfigTemplate), &templateDoc); err != nil {
		return fmt.Errorf("erro ao parsear template: %w", err)
	}

	// O documento YAML raiz tem Kind=DocumentNode e o conteúdo está em Content[0]
	if userDoc.Kind != yaml.DocumentNode || len(userDoc.Content) == 0 {
		return fmt.Errorf("config do usuário tem formato inesperado")
	}
	if templateDoc.Kind != yaml.DocumentNode || len(templateDoc.Content) == 0 {
		return fmt.Errorf("template tem formato inesperado")
	}

	userRoot := userDoc.Content[0]
	templateRoot := templateDoc.Content[0]

	// Faz o merge recursivo
	changed := mergeNodes(userRoot, templateRoot)

	if !changed {
		return nil
	}

	// Serializa de volta para YAML
	out, err := yaml.Marshal(&userDoc)
	if err != nil {
		return fmt.Errorf("erro ao serializar config migrado: %w", err)
	}

	// Adiciona marcadores YAML
	content := "---\n" + string(out) + "...\n"

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("erro ao salvar config migrado: %w", err)
	}

	fmt.Println("✓ Configuração atualizada com novas opções disponíveis.")
	return nil
}

// mergeNodes faz merge recursivo de nós YAML.
// Adiciona ao dst (usuário) chaves que existem em src (template) mas não em dst.
// Nunca sobrescreve valores existentes do usuário.
// Retorna true se houve alguma adição.
func mergeNodes(dst, src *yaml.Node) bool {
	// Só faz merge de mappings
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return false
	}

	changed := false

	// Percorre pares key/value do template (src)
	for i := 0; i < len(src.Content)-1; i += 2 {
		srcKey := src.Content[i]
		srcVal := src.Content[i+1]

		// Procura a chave no config do usuário (dst)
		dstIdx := findKeyIndex(dst, srcKey.Value)

		if dstIdx == -1 {
			// Chave não existe no config do usuário - adiciona
			keyCopy := copyNode(srcKey)
			valCopy := copyNode(srcVal)
			dst.Content = append(dst.Content, keyCopy, valCopy)
			changed = true
		} else {
			dstVal := dst.Content[dstIdx+1]
			if dstVal.Kind == yaml.MappingNode && srcVal.Kind == yaml.MappingNode {
				// Ambos são mappings - faz merge recursivo
				if mergeNodes(dstVal, srcVal) {
					changed = true
				}
			} else if dstVal.Kind == yaml.SequenceNode && srcVal.Kind == yaml.SequenceNode {
				// Ambos são listas - verifica campos faltantes nos itens existentes
				if mergeSequenceItems(dstVal, srcVal) {
					changed = true
				}
			}
		}
	}

	return changed
}

// mergeSequenceItems verifica itens de uma lista do usuário contra o "esquema" do template.
// Para cada item (mapping) da lista do usuário, adiciona campos que existem no template mas faltam no item.
// Campos adicionados recebem valores default vazios (ex: tags: []).
func mergeSequenceItems(dst, src *yaml.Node) bool {
	// Precisa ter pelo menos um item no template para extrair o esquema
	if len(src.Content) == 0 {
		return false
	}

	// Usa o primeiro item do template como referência de esquema
	templateItem := src.Content[0]
	if templateItem.Kind != yaml.MappingNode {
		return false
	}

	changed := false

	// Para cada item na lista do usuário
	for _, dstItem := range dst.Content {
		if dstItem.Kind != yaml.MappingNode {
			continue
		}

		// Verifica cada campo do template
		for i := 0; i < len(templateItem.Content)-1; i += 2 {
			tmplKey := templateItem.Content[i]
			tmplVal := templateItem.Content[i+1]

			// Se o campo não existe no item do usuário, adiciona com valor default vazio
			if findKeyIndex(dstItem, tmplKey.Value) == -1 {
				keyCopy := copyNode(tmplKey)
				valCopy := emptyValueNode(tmplVal)
				dstItem.Content = append(dstItem.Content, keyCopy, valCopy)
				changed = true
			}
		}
	}

	return changed
}

// emptyValueNode cria um nó yaml com valor default vazio baseado no tipo do nó de referência.
// Sequências viram [], mappings viram {}, escalares viram "".
func emptyValueNode(ref *yaml.Node) *yaml.Node {
	switch ref.Kind {
	case yaml.SequenceNode:
		return &yaml.Node{
			Kind:  yaml.SequenceNode,
			Tag:   "!!seq",
			Style: yaml.FlowStyle,
		}
	case yaml.MappingNode:
		return &yaml.Node{
			Kind:  yaml.MappingNode,
			Tag:   "!!map",
			Style: yaml.FlowStyle,
		}
	default:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: "",
		}
	}
}

// findKeyIndex procura uma chave em um MappingNode e retorna o índice dela.
// Retorna -1 se não encontrada.
func findKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// copyNode cria uma cópia profunda de um yaml.Node
func copyNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}

	cp := &yaml.Node{
		Kind:        node.Kind,
		Style:       node.Style,
		Tag:         node.Tag,
		Value:       node.Value,
		Anchor:      node.Anchor,
		HeadComment: node.HeadComment,
		LineComment: node.LineComment,
		FootComment: node.FootComment,
	}

	for _, child := range node.Content {
		cp.Content = append(cp.Content, copyNode(child))
	}

	return cp
}
