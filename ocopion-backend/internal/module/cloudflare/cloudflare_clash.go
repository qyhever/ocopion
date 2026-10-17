package cloudflare

import (
	"bytes"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

var clashSources = []string{"🇺🇸 US-CF-Vless-WS", "🇰🇷 KR-CF-Vless-WS"}
var clashPrefixes = []string{"🇺🇸 US-CFIP-Vless-WS", "🇰🇷 KR-CFIP-Vless-WS"}
var clashGroups = []string{"🚀 节点选择", "♻️ 自动选择", "♻️ 自动选择-US", "♻️ 自动选择-KR"}

func generateClash(template []byte, ips []string) ([]byte, error) {
	if len(ips) != fastIPCount {
		return nil, fmt.Errorf("生成 Clash 配置必须为 %d 个 IP", fastIPCount)
	}
	var doc yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(template))
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("解析 Clash 模板失败: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("Clash 模板必须只有一个 YAML 文档")
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("Clash 模板根节点必须为映射")
	}
	// Decode 校验重复键等结构错误，编辑和输出仍使用节点树以保留注释。
	var validated map[string]interface{}
	if err := doc.Decode(&validated); err != nil {
		return nil, fmt.Errorf("Clash 模板格式错误: %w", err)
	}
	root := doc.Content[0]
	proxies, groups := mappingValue(root, "proxies"), mappingValue(root, "proxy-groups")
	if proxies == nil || proxies.Kind != yaml.SequenceNode || groups == nil || groups.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("Clash 模板 proxies 和 proxy-groups 必须为列表")
	}
	nodes, err := namedNodes(proxies)
	if err != nil {
		return nil, err
	}
	groupNodes, err := namedNodes(groups)
	if err != nil {
		return nil, err
	}
	for name := range nodes {
		if groupNodes[name] != nil {
			return nil, fmt.Errorf("Clash 节点名称冲突: %s", name)
		}
	}
	members := make([]*yaml.Node, len(clashGroups))
	for i, name := range clashGroups {
		members[i] = mappingValue(groupNodes[name], "proxies")
		if members[i] == nil || members[i].Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("Clash 模板缺少策略组或成员列表: %s", name)
		}
		for _, member := range members[i].Content {
			if member.Kind != yaml.ScalarNode || member.Tag != "!!str" {
				return nil, fmt.Errorf("Clash 策略组 %s 成员必须为字符串", name)
			}
		}
	}
	for region, sourceName := range clashSources {
		source := nodes[sourceName]
		if source == nil || mappingValue(source, "server") == nil {
			return nil, fmt.Errorf("Clash 模板缺少原节点或 server: %s", sourceName)
		}
		for i, ip := range ips {
			name := fmt.Sprintf("%s%d", clashPrefixes[region], i+1)
			if nodes[name] != nil || groupNodes[name] != nil {
				return nil, fmt.Errorf("Clash 节点名称冲突: %s", name)
			}
			for _, list := range members {
				for _, member := range list.Content {
					if member.Value == name {
						return nil, fmt.Errorf("Clash 策略组已包含新增节点名称: %s", name)
					}
				}
			}
			copy := cloneYAML(source)
			setString(mappingValue(copy, "name"), name)
			server := mappingValue(copy, "server")
			if server.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("Clash 原节点 server 必须为标量: %s", sourceName)
			}
			setString(server, ip)
			proxies.Content = append(proxies.Content, copy)
			for _, group := range []int{0, 1, region + 2} {
				members[group].Content = append(members[group].Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name, Style: yaml.DoubleQuotedStyle})
			}
		}
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return nil, fmt.Errorf("编码 Clash 配置失败: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func namedNodes(list *yaml.Node) (map[string]*yaml.Node, error) {
	result := make(map[string]*yaml.Node)
	for _, node := range list.Content {
		name := mappingValue(node, "name")
		if name == nil || name.Kind != yaml.ScalarNode || name.Tag != "!!str" || name.Value == "" {
			return nil, fmt.Errorf("Clash 节点或策略组缺少有效 name")
		}
		if result[name.Value] != nil {
			return nil, fmt.Errorf("Clash 节点名称冲突: %s", name.Value)
		}
		result[name.Value] = node
	}
	return result, nil
}

func setString(node *yaml.Node, value string) {
	node.Value = value
	node.Tag = "!!str"
}

// 新节点展开别名并移除锚点，避免复制后重复声明锚点或共享可变字段。
func cloneYAML(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode {
		return cloneYAML(node.Alias)
	}
	copy := *node
	copy.Anchor = ""
	copy.Content = nil
	for _, child := range node.Content {
		copy.Content = append(copy.Content, cloneYAML(child))
	}
	return &copy
}
