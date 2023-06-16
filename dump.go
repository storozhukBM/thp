package thp

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
)

var lineDescriptorsCache = newPCache(64)

// Dump will simplify local print debug.
// Just pass here all the variable that you want to dump to stdout.
func Dump(values ...interface{}) {
	if len(values) == 0 {
		return
	}
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		fmt.Printf("[DEBUG] can't capture stack for dump\n")
		return
	}
	descriptor, ok := lineDescriptorsCache.load(cacheKey{file: file, line: line})
	if !ok {
		lineDescriptor, err := describeLine(file, line)
		if err != nil {
			fmt.Printf("[DEBUG] %v\n", err)
			return
		}
		lineDescriptorsCache.store(cacheKey{file: file, line: line}, lineDescriptor)
		descriptor = lineDescriptor
	}

	dumpDataToStdOut(file, line, descriptor.targetLine, descriptor.dumpVariables, values)
}

func describeLine(file string, line int) (lineDescriptor, error) {
	targetFile, openErr := os.Open(file)
	if openErr != nil {
		return lineDescriptor{}, fmt.Errorf("can't open file: %v\n", file)
	}
	defer func() {
		err := targetFile.Close()
		if err != nil {
			fmt.Printf("[DEBUG] can't close file: %v; err: %v\n", file, err)
		}
	}()

	scanner := bufio.NewScanner(targetFile)
	lineCnt := 0
	targetLine := ""
	for scanner.Scan() {
		lineCnt++
		if lineCnt != line {
			continue
		}
		fileLine := strings.Trim(scanner.Text(), ` 	`)
		dumpStartIdx := strings.Index(fileLine, "Dump(")
		dumpEndIdx := strings.LastIndex(fileLine, ")")
		if dumpStartIdx < 0 || dumpEndIdx < 0 {
			return lineDescriptor{}, fmt.Errorf(
				"target line is invalid. Dump should start with `Dump(` and end with `)`: %v\n",
				fileLine,
			)
		}
		targetLine = fileLine[dumpStartIdx+5 : dumpEndIdx]
		break
	}
	dumpVariables := strings.Split(targetLine, ", ")
	return lineDescriptor{targetLine: targetLine, dumpVariables: dumpVariables}, nil
}

func dumpDataToStdOut(file string, line int, targetLine string, dumpVariables []string, values []interface{}) {
	if len(dumpVariables) != len(values) {
		buff := &bytes.Buffer{}
		_, _ = fmt.Fprintf(buff, "[DEBUG] %v:%v: ", file, line)
		_, _ = fmt.Fprintf(buff, "%v: ", targetLine)
		for idx, val := range values {
			_, _ = fmt.Fprintf(buff, "`%+v`", val)
			if idx < len(values)-1 {
				_, _ = fmt.Fprintf(buff, "; ")
			}
		}
		_, _ = fmt.Fprintf(buff, "\n")
		fmt.Print(buff.String())
		return
	}

	buff := &bytes.Buffer{}
	_, _ = fmt.Fprintf(buff, "[DEBUG] %v:%v: ", file, line)
	for idx, variable := range dumpVariables {
		isStringLiteral := strings.HasPrefix(variable, `"`) && strings.HasSuffix(variable, `"`)
		isStringLiteral = isStringLiteral || strings.HasPrefix(variable, "`") && strings.HasSuffix(variable, "`")
		if isStringLiteral {
			_, _ = fmt.Fprintf(buff, "%v", variable[1:len(variable)-1])
		} else {
			_, _ = fmt.Fprintf(buff, "%v: `%+v`", variable, values[idx])
			if idx < len(values)-1 {
				_, _ = fmt.Fprintf(buff, "; ")
			}
		}
	}
	_, _ = fmt.Fprintf(buff, "\n")
	fmt.Print(buff.String())
}

type lineDescriptor struct {
	targetLine    string
	dumpVariables []string
}

type cacheKey struct {
	file string
	line int
}

type cacheStripe struct {
	cache map[cacheKey]lineDescriptor
}

type pCache struct {
	maxSize int
	pool    *sync.Pool
}

// newPCache creates pCache with maxSizePerGoroutine.
func newPCache(maxSizePerGoroutine uint) *pCache {
	return &pCache{
		maxSize: int(maxSizePerGoroutine),
		pool: &sync.Pool{
			New: func() interface{} {
				return &cacheStripe{
					cache: make(map[cacheKey]lineDescriptor),
				}
			},
		},
	}
}

// load fetches (value, true) from cache associated with key or (lineDescriptor{}, false) if it is not present.
func (p *pCache) load(key cacheKey) (lineDescriptor, bool) {
	stripe := p.pool.Get().(*cacheStripe)
	defer p.pool.Put(stripe)
	value, ok := stripe.cache[key]
	return value, ok
}

// store stores value for a key in cache.
func (p *pCache) store(key cacheKey, value lineDescriptor) {
	stripe := p.pool.Get().(*cacheStripe)
	defer p.pool.Put(stripe)

	stripe.cache[key] = value
	if len(stripe.cache) <= p.maxSize {
		return
	}
	for k := range stripe.cache {
		delete(stripe.cache, k)
		break
	}
}
