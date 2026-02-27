package pkgs

// ImportPkgs 定义需要自动生成注册代码的标准库包列表。
// key 为包的导入路径，value 为额外的导入依赖。
// 如果是 vendor 目录下的包，路径需要加上 vendor/ 前缀。
// 注释掉的包可按需取消注释以启用。
var ImportPkgs = map[string][]string{
	"bytes": []string{},
	"container/heap":  []string{},
	"container/list":  []string{},
	"container/ring":  []string{},
	"crypto/md5":      []string{},
	"encoding/base64": []string{},
	"encoding/hex":    []string{},
	"encoding/xml":    []string{},
	"errors":          []string{},
	"fmt": []string{},
	"html":          []string{},
	"math": []string{},
	"math/rand":     []string{},
	"net/http":      []string{},
	"net/url":       []string{},
	"regexp":        []string{},
	"sort":          []string{},
	"strconv":       []string{},
	"strings": []string{},
	"time":    []string{},
	"unicode":       []string{},
	"unicode/utf8":  []string{},
	"unicode/utf16": []string{},
	"sync":          []string{},
	"sync/atomic":   []string{},

	"crypto/sha1":     []string{},
	"encoding/json":   []string{},
	"encoding/binary": []string{},
	"io/ioutil":       []string{"io"},
	"io":              []string{},
	"html/template":   []string{},
	"path":            []string{},
	"mime/multipart":  []string{},
	"crypto/des":      []string{},
	"crypto/cipher":   []string{},
	"crypto/tls":      []string{},
}
