package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexeiev/sshControl/config"
)

func TestResolveHostInputsFromFile(t *testing.T) {
	t.Parallel()

	cfg := &config.ConfigFile{
		Hosts: []config.Host{
			{Name: "web1", Host: "10.0.0.1", Port: 22, Tags: []string{"web"}},
			{Name: "web2", Host: "10.0.0.2", Port: 22, Tags: []string{"web"}},
			{Name: "api", Host: "10.0.0.3", Port: 22},
		},
	}

	tempDir := t.TempDir()
	listPath := filepath.Join(tempDir, "lista.txt")
	content := "10.10.10.10, 10.10.10.11;\n@web\napi\n"
	if err := os.WriteFile(listPath, []byte(content), 0644); err != nil {
		t.Fatalf("falha ao criar lista: %v", err)
	}

	gotHosts, gotTags, err := ResolveHostInputs(cfg, []string{listPath})
	if err != nil {
		t.Fatalf("ResolveHostInputs retornou erro: %v", err)
	}

	wantHosts := []string{"10.10.10.10", "10.10.10.11", "web1", "web2", "api"}
	if !reflect.DeepEqual(gotHosts, wantHosts) {
		t.Fatalf("hosts incorretos: got=%v want=%v", gotHosts, wantHosts)
	}

	wantTags := []string{"web"}
	if !reflect.DeepEqual(gotTags, wantTags) {
		t.Fatalf("tags incorretas: got=%v want=%v", gotTags, wantTags)
	}
}

func TestIsHostListFileRejectsRegularScript(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "script.sh")
	script := "#!/bin/bash\necho oi\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("falha ao criar script: %v", err)
	}

	if IsHostListFile(&config.ConfigFile{}, scriptPath) {
		t.Fatalf("arquivo comum foi identificado incorretamente como lista de hosts")
	}
}

func TestParseMultipleUploadArgsWithHostFile(t *testing.T) {
	t.Parallel()

	cfg := &config.ConfigFile{}
	tempDir := t.TempDir()

	listPath := filepath.Join(tempDir, "lista.txt")
	if err := os.WriteFile(listPath, []byte("10.10.10.10\n10.10.10.11\n"), 0644); err != nil {
		t.Fatalf("falha ao criar lista: %v", err)
	}

	localFile := filepath.Join(tempDir, "script.sh")
	if err := os.WriteFile(localFile, []byte("#!/bin/bash\necho ok\n"), 0755); err != nil {
		t.Fatalf("falha ao criar arquivo local: %v", err)
	}

	hostArgs, localPath, remotePath, err := ParseMultipleUploadArgs(cfg, []string{listPath, localFile, "/opt/app/"})
	if err != nil {
		t.Fatalf("ParseMultipleUploadArgs retornou erro: %v", err)
	}

	if !reflect.DeepEqual(hostArgs, []string{listPath}) {
		t.Fatalf("hosts incorretos: got=%v", hostArgs)
	}
	if localPath != localFile {
		t.Fatalf("localPath incorreto: got=%s want=%s", localPath, localFile)
	}
	if remotePath != "/opt/app/" {
		t.Fatalf("remotePath incorreto: got=%s", remotePath)
	}
}

func TestParseMultipleUploadArgsWithMultipleHostFiles(t *testing.T) {
	t.Parallel()

	cfg := &config.ConfigFile{}
	tempDir := t.TempDir()

	listA := filepath.Join(tempDir, "lista-a.txt")
	if err := os.WriteFile(listA, []byte("10.10.10.10\n"), 0644); err != nil {
		t.Fatalf("falha ao criar lista A: %v", err)
	}

	listB := filepath.Join(tempDir, "lista-b.txt")
	if err := os.WriteFile(listB, []byte("10.10.10.11;10.10.10.12\n"), 0644); err != nil {
		t.Fatalf("falha ao criar lista B: %v", err)
	}

	localDir := filepath.Join(tempDir, "dist")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("falha ao criar diretório local: %v", err)
	}

	hostArgs, localPath, remotePath, err := ParseMultipleUploadArgs(cfg, []string{listA, listB, localDir})
	if err != nil {
		t.Fatalf("ParseMultipleUploadArgs retornou erro: %v", err)
	}

	wantHosts := []string{listA, listB}
	if !reflect.DeepEqual(hostArgs, wantHosts) {
		t.Fatalf("hosts incorretos: got=%v want=%v", hostArgs, wantHosts)
	}
	if localPath != localDir {
		t.Fatalf("localPath incorreto: got=%s want=%s", localPath, localDir)
	}
	if remotePath != "~" {
		t.Fatalf("remotePath incorreto: got=%s want=%s", remotePath, "~")
	}
}
