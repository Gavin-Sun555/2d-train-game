package game

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
	"github.com/Gavin-Sun555/2d-train-game/street"
)

var cjk = regexp.MustCompile(`[\p{Han}\x{3000}-\x{303f}\x{ff00}-\x{ffef}]`)

// Every piece of text written as locale.T(zh, en) or locale.L(zh, en) has
// an English half with no Chinese in it, and the same format verbs.
func TestEnglishTextIsEnglish(t *testing.T) {
	verbs := regexp.MustCompile(`%[-+ #0]*[0-9.]*[a-zA-Z]`)
	n := 0
	for _, dir := range []string{".", "../street"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || len(call.Args) != 2 {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "locale" || (sel.Sel.Name != "T" && sel.Sel.Name != "L") {
					return true
				}
				zh, en := strs(call.Args[0]), strs(call.Args[1])
				if len(zh) != len(en) {
					t.Errorf("%s: %d Chinese texts, %d English", f, len(zh), len(en))
					return true
				}
				for i := range en {
					n++
					if en[i] == "" && zh[i] != "" || cjk.MatchString(en[i]) {
						t.Errorf("%s: English for %q is %q", f, zh[i], en[i])
					}
					if a, b := verbs.FindAllString(zh[i], -1), verbs.FindAllString(en[i], -1); strings.Join(a, " ") != strings.Join(b, " ") {
						t.Errorf("%s: %q has %v, English %q has %v", f, zh[i], a, en[i], b)
					}
				}
				return true
			})
		}
	}
	if n < 250 {
		t.Errorf("only %d texts found", n)
	}
}

// strs is the string literals in a literal or a []string{...}.
func strs(e ast.Expr) []string {
	switch v := e.(type) {
	case *ast.BasicLit:
		s, _ := strconv.Unquote(v.Value)
		return []string{s}
	case *ast.CompositeLit:
		var out []string
		for _, el := range v.Elts {
			out = append(out, strs(el)...)
		}
		return out
	}
	return nil
}

// In English the interface's fixed words and the landmarks' names are
// English; a renamed landmark keeps the player's name.
func TestEnglishNames(t *testing.T) {
	locale.Current = locale.English
	defer func() { locale.Current = locale.Chinese }()
	for _, list := range [][]string{pageNames(), toolNames(), levelNames(), passNames(), layoutShort(), roadModeNames(), levelHints()} {
		for _, s := range list {
			if cjk.MatchString(s) {
				t.Errorf("%q in English", s)
			}
		}
	}
	for i := range layouts {
		if cjk.MatchString(layoutName(i)) || cjk.MatchString(laneOptName(min(i, len(roadLaneOpts)-1))) {
			t.Errorf("layout %d not English", i)
		}
	}
	for _, m := range maps {
		if cjk.MatchString(m.title()) || cjk.MatchString(m.desc()) {
			t.Errorf("map %s not English", m.key)
		}
	}
	for _, tm := range street.TramModels {
		if cjk.MatchString(tm.Name()) || cjk.MatchString(tm.ShortName()) || cjk.MatchString(tm.Region()) {
			t.Errorf("tram model %s not English: %q / %q / %q", tm.Key, tm.Name(), tm.ShortName(), tm.Region())
		}
	}
	g := New("seattle")
	for i, l := range g.scene.landmarks {
		if got := g.LandmarkName(i); got != l.key {
			t.Errorf("landmark %d shows %q in English, want %q", i, got, l.key)
		}
	}
	if err := g.RenameLandmark(0, "My place"); err != nil || g.LandmarkName(0) != "My place" {
		t.Error("renamed landmark")
	}
	// Messages from the street network are English too.
	p := g.net.Junctions[0].Pos
	if err := g.net.AddRoad(p, p, 1, 1, 1); err == nil || cjk.MatchString(err.Error()) {
		t.Errorf("street error %v", err)
	}
}
