package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	asarBlockSize = 4 * 1024 * 1024
	startMarker   = "/*@ND_ADBLOCK_V1_START@*/"
	endMarker     = "/*@ND_ADBLOCK_V1_END@*/"
)

type asarEntry struct {
	Path string
	Node map[string]any
}

func parseAsar(buf []byte) (map[string]any, []byte, error) {
	if len(buf) < 16 {
		return nil, nil, fmt.Errorf("asar 太小")
	}
	jlen := int(binary.LittleEndian.Uint32(buf[12:16]))
	if jlen <= 0 || 16+jlen > len(buf) {
		return nil, nil, fmt.Errorf("asar 头长度异常: %d", jlen)
	}
	dec := json.NewDecoder(bytes.NewReader(buf[16 : 16+jlen]))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return nil, nil, fmt.Errorf("asar JSON 解析失败: %w", err)
	}
	dataStart := 16 + ((jlen + 3) &^ 3)
	if dataStart > len(buf) {
		return nil, nil, fmt.Errorf("asar 数据起点越界")
	}
	return root, buf[dataStart:], nil
}

func walkAsar(m map[string]any, prefix string, out *[]asarEntry) {
	files, _ := m["files"].(map[string]any)
	for name, v := range files {
		node, ok := v.(map[string]any)
		if !ok {
			continue
		}
		p := name
		if prefix != "" {
			p = prefix + "/" + name
		}
		if _, isDir := node["files"].(map[string]any); isDir {
			walkAsar(node, p, out)
		} else {
			*out = append(*out, asarEntry{Path: p, Node: node})
		}
	}
}

func packedEntries(root map[string]any) []asarEntry {
	var all []asarEntry
	walkAsar(root, "", &all)
	var out []asarEntry
	for _, e := range all {
		off, ok := e.Node["offset"].(string)
		if !ok || off == "" {
			continue
		}
		if u, ok := e.Node["unpacked"].(bool); ok && u {
			continue
		}
		out = append(out, e)
	}
	// 按 offset 排序；offset 都是十进制字符串
	sortEntries(out)
	return out
}

func sortEntries(es []asarEntry) {
	// 简单的插入排序，避免引入 sort 依赖的额外比较函数
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && offsetOf(es[j-1]) > offsetOf(es[j]); j-- {
			es[j-1], es[j] = es[j], es[j-1]
		}
	}
}

func offsetOf(e asarEntry) int64 {
	s, _ := e.Node["offset"].(string)
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		return n
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	}
	return 0
}

func readEntry(data []byte, node map[string]any) ([]byte, error) {
	off := offsetOf(asarEntry{Node: node})
	size := asInt64(node["size"])
	if off < 0 || size < 0 || off+size > int64(len(data)) {
		return nil, fmt.Errorf("条目越界 offset=%d size=%d data=%d", off, size, len(data))
	}
	return data[off : off+size], nil
}

func entryNode(root map[string]any, path string) (map[string]any, bool) {
	cur := root
	parts := strings.Split(path, "/")
	for i, p := range parts {
		files, _ := cur["files"].(map[string]any)
		v, ok := files[p]
		if !ok {
			return nil, false
		}
		node, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return node, true
		}
		cur = node
	}
	return nil, false
}

func sha256hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func integrityOf(b []byte) map[string]any {
	blocks := []string{}
	for i := 0; i < len(b); i += asarBlockSize {
		end := i + asarBlockSize
		if end > len(b) {
			end = len(b)
		}
		blocks = append(blocks, sha256hex(b[i:end]))
	}
	return map[string]any{
		"algorithm": "SHA256",
		"hash":      sha256hex(b),
		"blockSize": asarBlockSize,
		"blocks":    blocks,
	}
}

func versionOf(root map[string]any, data []byte) string {
	node, ok := entryNode(root, "version.json")
	if !ok {
		return "unknown"
	}
	b, err := readEntry(data, node)
	if err != nil {
		return "unknown"
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return "unknown"
	}
	return fmt.Sprintf("%v.%v", v["version"], v["fourth"])
}

func insertLoader(main, loader []byte) []byte {
	block := []byte(startMarker + "\n" + string(loader) + "\n" + endMarker + "\n")
	pos := bytes.Index(main, []byte("/*! For license information please see main.js.LICENSE.txt */"))
	if pos < 0 {
		pos = 0
	}
	out := make([]byte, 0, len(main)+len(block))
	out = append(out, main[:pos]...)
	out = append(out, block...)
	out = append(out, main[pos:]...)
	return out
}

// buildPatched 返回 patched asar、版本号、原始条目快照（用于自检）
func buildPatched(src, mainNew []byte) ([]byte, string, map[string][]byte, error) {
	root, data, err := parseAsar(src)
	if err != nil {
		return nil, "", nil, err
	}
	version := versionOf(root, data)
	entries := packedEntries(root)
	if len(entries) == 0 {
		return nil, "", nil, fmt.Errorf("asar 里没有可打包条目")
	}

	orig := map[string][]byte{}
	var sum int64
	for _, e := range entries {
		b, err := readEntry(data, e.Node)
		if err != nil {
			return nil, "", nil, err
		}
		orig[e.Path] = b
		sum += int64(len(b))
	}
	if sum != int64(len(data)) {
		return nil, "", nil, fmt.Errorf("asar 数据长度不一致: entries=%d data=%d", sum, len(data))
	}

	foundMain := false
	var parts [][]byte
	cursor := 0
	for _, e := range entries {
		var content []byte
		if e.Path == "main.js" {
			content = mainNew
			foundMain = true
			e.Node["size"] = int64(len(content))
			e.Node["integrity"] = integrityOf(content)
		} else {
			content = orig[e.Path]
		}
		e.Node["offset"] = strconv.Itoa(cursor)
		parts = append(parts, content)
		cursor += len(content)
	}
	if !foundMain {
		return nil, "", nil, fmt.Errorf("asar 里没有 main.js")
	}

	jsonBytes, err := json.Marshal(root)
	if err != nil {
		return nil, "", nil, err
	}
	jlen := len(jsonBytes)
	aligned := (jlen + 3) &^ 3
	head := make([]byte, 16+aligned)
	binary.LittleEndian.PutUint32(head[0:], 4)
	binary.LittleEndian.PutUint32(head[4:], uint32(8+aligned))
	binary.LittleEndian.PutUint32(head[8:], uint32(4+aligned))
	binary.LittleEndian.PutUint32(head[12:], uint32(jlen))
	copy(head[16:], jsonBytes)

	var buf bytes.Buffer
	buf.Grow(16 + aligned + cursor)
	buf.Write(head)
	for _, p := range parts {
		buf.Write(p)
	}
	return buf.Bytes(), version, orig, nil
}

func verifyPatched(patched []byte) error {
	root, data, err := parseAsar(patched)
	if err != nil {
		return err
	}
	node, ok := entryNode(root, "main.js")
	if !ok {
		return fmt.Errorf("缺少 main.js")
	}
	main, err := readEntry(data, node)
	if err != nil {
		return err
	}
	if !bytes.Contains(main, []byte(startMarker)) {
		return fmt.Errorf("缺少注入标记")
	}
	integ, _ := node["integrity"].(map[string]any)
	hash, _ := integ["hash"].(string)
	if hash != sha256hex(main) {
		return fmt.Errorf("main.js integrity 不匹配")
	}
	return nil
}

func selfCheck(out []byte, orig map[string][]byte) error {
	root, data, err := parseAsar(out)
	if err != nil {
		return err
	}
	main, err := readEntry(data, mustNode(root, "main.js"))
	if err != nil {
		return err
	}
	if !bytes.Contains(main, []byte(startMarker)) {
		return fmt.Errorf("自检失败: 缺少注入标记")
	}
	for _, e := range packedEntries(root) {
		if e.Path == "main.js" {
			continue
		}
		b, err := readEntry(data, e.Node)
		if err != nil {
			return err
		}
		o, ok := orig[e.Path]
		if !ok || !bytes.Equal(b, o) {
			return fmt.Errorf("自检失败: 条目被改动 %s", e.Path)
		}
	}
	return nil
}

func mustNode(root map[string]any, path string) map[string]any {
	n, _ := entryNode(root, path)
	return n
}

func readMainFromAsar(src []byte) ([]byte, error) {
	root, data, err := parseAsar(src)
	if err != nil {
		return nil, err
	}
	node, ok := entryNode(root, "main.js")
	if !ok {
		return nil, fmt.Errorf("缺少 main.js")
	}
	return readEntry(data, node)
}