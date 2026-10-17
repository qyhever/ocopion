package cloudflare

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func templateFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/clash-template.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func testIPs() []string {
	var ips []string
	for i := 1; i <= fastIPCount; i++ {
		ips = append(ips, fmt.Sprintf("1.1.1.%d", i))
	}
	return ips
}
func decodeDocument(t *testing.T, data []byte) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}
func decodedValue(t *testing.T, node *yaml.Node) interface{} {
	t.Helper()
	var value interface{}
	if err := node.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestGenerateClash(t *testing.T) {
	template := templateFixture(t)
	before := decodeDocument(t, template)
	ips := testIPs()
	output, err := generateClash(template, ips)
	if err != nil {
		t.Fatal(err)
	}
	again, err := generateClash(template, ips)
	if err != nil || !bytes.Equal(output, again) {
		t.Fatal("generation is not repeatable")
	}
	after := decodeDocument(t, output)
	original := mappingValue(before, "proxies")
	proxies := mappingValue(after, "proxies")
	if len(original.Content) != 2 || len(proxies.Content) != 34 {
		t.Fatalf("wrong node counts: %d %d", len(original.Content), len(proxies.Content))
	}
	for i, node := range original.Content {
		if !reflect.DeepEqual(decodedValue(t, node), decodedValue(t, proxies.Content[i])) {
			t.Fatal("original proxy changed")
		}
	}
	for region := range clashSources {
		for i, ip := range ips {
			node := proxies.Content[2+region*16+i]
			name := fmt.Sprintf("%s%d", clashPrefixes[region], i+1)
			if mappingValue(node, "name").Value != name || mappingValue(node, "server").Value != ip {
				t.Fatalf("wrong generated proxy: %s", name)
			}
			setString(mappingValue(node, "name"), clashSources[region])
			setString(mappingValue(node, "server"), mappingValue(original.Content[region], "server").Value)
			if !reflect.DeepEqual(decodedValue(t, node), decodedValue(t, original.Content[region])) {
				t.Fatalf("proxy fields changed: %s", name)
			}
		}
	}
	oldGroups, _ := namedNodes(mappingValue(before, "proxy-groups"))
	newGroups, _ := namedNodes(mappingValue(after, "proxy-groups"))
	for name, group := range oldGroups {
		old := mappingValue(group, "proxies")
		current := mappingValue(newGroups[name], "proxies")
		expected := append([]*yaml.Node{}, old.Content...)
		for region := range clashSources {
			if name == clashGroups[0] || name == clashGroups[1] || name == clashGroups[region+2] {
				for i := range ips {
					expected = append(expected, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprintf("%s%d", clashPrefixes[region], i+1)})
				}
			}
		}
		if len(current.Content) != len(expected) {
			t.Fatalf("group %s count mismatch", name)
		}
		for i := range expected {
			if current.Content[i].Value != expected[i].Value {
				t.Fatalf("group %s member mismatch", name)
			}
		}
		// 成员之外的策略组属性也应保持不变。
		old.Content, current.Content = nil, nil
		if !reflect.DeepEqual(decodedValue(t, group), decodedValue(t, newGroups[name])) {
			t.Fatalf("group properties changed: %s", name)
		}
	}
	for i := 0; i < len(before.Content); i += 2 {
		key := before.Content[i].Value
		if key == "proxies" || key == "proxy-groups" {
			continue
		}
		if !reflect.DeepEqual(decodedValue(t, before.Content[i+1]), decodedValue(t, mappingValue(after, key))) {
			t.Fatalf("config changed: %s", key)
		}
	}
	for _, comment := range []string{"# 手动选择", "# 自动测速选择"} {
		if !bytes.Contains(output, []byte(comment)) {
			t.Fatalf("comment lost: %s", comment)
		}
	}
}

func TestBadClashTemplates(t *testing.T) {
	template := string(templateFixture(t))
	tests := map[string]string{
		"syntax":             "proxies: [",
		"empty":              "",
		"root":               "[]",
		"multiple documents": template + "\n---\nfoo: bar\n",
		"duplicate keys":     template + "\nmode: global\n",
		"missing proxies":    strings.Replace(template, "\nproxies:", "\nother-proxies:", 1),
		"bad groups":         strings.Replace(template, "proxy-groups:", "proxy-groups: invalid\nother-groups:", 1),
		"missing US":         strings.ReplaceAll(template, clashSources[0], "other-US"),
		"missing KR":         strings.ReplaceAll(template, clashSources[1], "other-KR"),
		"duplicate name":     strings.ReplaceAll(template, clashSources[1], clashSources[0]),
		"missing server":     strings.Replace(template, "    server:", "    other-server:", 1),
		"name conflict":      strings.ReplaceAll(template, clashSources[0], clashPrefixes[0]+"1"),
	}
	for _, name := range clashGroups {
		tests["missing "+name] = strings.ReplaceAll(template, name, "other-"+name)
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := generateClash([]byte(data), testIPs()); err == nil {
				t.Fatal("accepted invalid template")
			}
		})
	}
	if _, err := generateClash([]byte(template), testIPs()[:15]); err == nil {
		t.Fatal("accepted insufficient IPs")
	}
	// 原节点存在时仍应检测新增名称和其他节点/策略组的冲突。
	for _, section := range []string{"proxies", "proxy-groups"} {
		doc := decodeDocument(t, []byte(template))
		list := mappingValue(doc, section)
		extra := cloneYAML(list.Content[0])
		setString(mappingValue(extra, "name"), clashPrefixes[0]+"1")
		list.Content = append(list.Content, extra)
		data, _ := yaml.Marshal(doc)
		if _, err := generateClash(data, testIPs()); err == nil || !strings.Contains(err.Error(), "名称冲突") {
			t.Fatalf("missing conflict for %s: %v", section, err)
		}
	}
}

func TestClashPaths(t *testing.T) {
	cwd, _ := os.Getwd()
	defaults := NewRepository(nil)
	if defaults.templatePath != filepath.Join(cwd, "internal/data/clash-template.yaml") || defaults.clashPath != filepath.Join(cwd, "public/clash-kdd.yaml") {
		t.Fatal("wrong default paths")
	}
	for _, which := range []string{"template", "IP output", "cfst input", "symlink parent", "hardlink"} {
		t.Run(which, func(t *testing.T) {
			repo := mockRepo(t, successScript)
			switch which {
			case "template":
				repo.clashPath = repo.templatePath
			case "IP output":
				repo.clashPath = repo.outputPath
			case "cfst input":
				repo.clashPath = filepath.Join(filepath.Dir(repo.binPath), "ip.txt")
			case "symlink parent":
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(filepath.Dir(repo.templatePath), link); err != nil {
					t.Fatal(err)
				}
				repo.clashPath = filepath.Join(link, filepath.Base(repo.templatePath))
			case "hardlink":
				repo.clashPath = filepath.Join(t.TempDir(), "hardlink")
				if err := os.Link(repo.templatePath, repo.clashPath); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := repo.ReadClashTemplate(); err == nil || !strings.Contains(err.Error(), "路径冲突") {
				t.Fatalf("missing path conflict: %v", err)
			}
			if err := repo.SaveClash([]byte("new")); err == nil {
				t.Fatal("unsafe Clash save allowed")
			}
			if err := repo.Save(testIPs()); err == nil {
				t.Fatal("unsafe IP save allowed")
			}
		})
	}
}

func TestClashTaskFailures(t *testing.T) {
	for _, scenario := range []string{"missing template", "bad template", "save failure"} {
		t.Run(scenario, func(t *testing.T) {
			repo := mockRepo(t, successScript)
			writeFile(t, filepath.Join(filepath.Dir(repo.binPath), "gate"), "", 0644)
			if err := os.MkdirAll(filepath.Dir(repo.outputPath), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(repo.clashPath), 0755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, repo.outputPath, "old IP", 0644)
			writeFile(t, repo.clashPath, "old Clash", 0644)
			oldClash := repo.clashPath
			switch scenario {
			case "missing template":
				os.Remove(repo.templatePath)
			case "bad template":
				writeFile(t, repo.templatePath, "invalid: [", 0644)
			case "save failure":
				// 父目录是普通文件时，创建输出失败并保留既有 Clash 文件。
				repo.clashPath = filepath.Join(oldClash, "clash.yaml")
			}
			service := NewService(repo)
			service.Start()
			task := waitTask(t, service)
			if task.Status != "failed" || len(task.IPs) != 0 || task.Error == "" {
				t.Fatalf("unexpected task: %+v", task)
			}
			data, _ := os.ReadFile(oldClash)
			if string(data) != "old Clash" {
				t.Fatal("old Clash changed")
			}
			data, _ = os.ReadFile(repo.outputPath)
			if scenario == "save failure" {
				if string(data) != strings.Join(testIPs(), "\n")+"\n" {
					t.Fatal("IP output should already be saved")
				}
			} else if string(data) != "old IP" {
				t.Fatal("IP changed before valid generation")
			}
		})
	}
	repo := mockRepo(t, successScript)
	repo.clashPath = t.TempDir()
	writeFile(t, filepath.Join(repo.clashPath, "old"), "old Clash", 0644)
	if err := repo.SaveClash([]byte("new")); err == nil {
		t.Fatal("expected rename failure")
	}
	entries, _ := os.ReadDir(filepath.Dir(repo.clashPath))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".ocopion-") {
			t.Fatal("temporary file leaked")
		}
	}
	data, _ := os.ReadFile(filepath.Join(repo.clashPath, "old"))
	if string(data) != "old Clash" {
		t.Fatal("old Clash changed after rename failure")
	}
}

type gatedClashRepository struct {
	*FileRepository
	entered chan struct{}
	release chan struct{}
}

func (r *gatedClashRepository) SaveClash(data []byte) error {
	select {
	case <-r.entered:
	default:
		close(r.entered)
	}
	<-r.release
	return r.FileRepository.SaveClash(data)
}
func TestTaskWaitsForClashSave(t *testing.T) {
	repo := mockRepo(t, successScript)
	writeFile(t, filepath.Join(filepath.Dir(repo.binPath), "gate"), "", 0644)
	gated := &gatedClashRepository{FileRepository: repo, entered: make(chan struct{}), release: make(chan struct{})}
	service := NewService(gated)
	service.Start()
	select {
	case <-gated.entered:
	case <-time.After(5 * time.Second):
		close(gated.release)
		t.Fatal("did not reach Clash save")
	}
	if service.Get().Status != "running" {
		close(gated.release)
		t.Fatal("task succeeded before Clash save")
	}
	close(gated.release)
	if waitTask(t, service).Status != "succeeded" {
		t.Fatal("task failed")
	}
	data, err := os.ReadFile(repo.clashPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(mappingValue(decodeDocument(t, data), "proxies").Content) != 34 {
		t.Fatal("bad saved config")
	}
	service.Start()
	if waitTask(t, service).Status != "succeeded" {
		t.Fatal("restart failed")
	}
	data, _ = os.ReadFile(repo.clashPath)
	if len(mappingValue(decodeDocument(t, data), "proxies").Content) != 34 {
		t.Fatal("restart accumulated nodes")
	}
}
