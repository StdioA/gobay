package main

import (
	"bytes"
	"embed"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"
)

type _projTemplate struct {
	content []byte
	dstPath string
	mode    fs.FileMode
}

type _projDir struct {
	dstPath string
	mode    fs.FileMode
}

type _projConfig struct {
	Url            string
	Name           string
	SkipSentry     bool
	SkipAsyncTask  bool
	SkipCache      bool
	SkipElasticApm bool
}

var (
	//go:embed templates/*
	tmplFS        embed.FS
	projDirs      = []_projDir{}
	projTemplates = []_projTemplate{}
	projConfig    = _projConfig{}
	projRawTmpl   = map[string]string{}
	tmplFuncs     = template.FuncMap{
		"toCamel":      strcase.ToCamel,
		"toLowerCamel": strcase.ToLowerCamel,
		"toSnake":      strcase.ToSnake,
	}
)

const (
	TMPLSUFFIX                = ".tmpl"
	RAW_TMPL_DIR              = "enttmpl"
	DIRMODE       fs.FileMode = fs.ModeDir | 0755
	FILEMODE      fs.FileMode = 0644
	WRITEMODEMASK fs.FileMode = 0200
	TMPLBASEDIR               = "templates"
)

func main() {
	cmd := &cobra.Command{Use: "gobay"}
	cmdNew := &cobra.Command{
		Use: "new [projectURL]",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				check(cmd.Help())
				return
			}
			url := args[0]
			url = strings.TrimSuffix(url, "/")
			projConfig.Url = url
			if projConfig.Name == "" {
				strs := strings.Split(url, "/")
				projConfig.Name = strs[len(strs)-1]
			}
			newProject()
		},
		Short: "initialize new gobay project",
		Long:  "Example: `gobay new github.com/shanbay/project`",
	}
	cmdNew.Flags().StringVar(&projConfig.Name, "name", "", "specific project name")
	cmdNew.Flags().BoolVar(&projConfig.SkipSentry, "skip-sentry", false, "skip sentry")
	cmdNew.Flags().BoolVar(&projConfig.SkipElasticApm, "skip-elasticapm", false, "skip elastic APM")
	cmdNew.Flags().BoolVar(&projConfig.SkipCache, "skip-cache", false, "skip cache")
	cmdNew.Flags().BoolVar(&projConfig.SkipAsyncTask, "skip-asynctask", false, "skip asynctask")

	cmd.AddCommand(cmdNew)
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}

func newProject() {
	if err := os.Mkdir(projConfig.Name, DIRMODE); os.IsExist(err) {
		log.Fatalf("already exists: %v", projConfig.Name)
	}

	// load
	if err := loadTemplates(); err != nil {
		log.Fatalln(err)
	}

	// render
	renderTemplates()

	// copy
	copyTmplFiles()
}

// loadTemplates loads templates and directory structure.
func loadTemplates() error {
	if err := fs.WalkDir(
		tmplFS,
		TMPLBASEDIR,
		func(filePath string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relpath, err := filepath.Rel(TMPLBASEDIR, filePath)
			if err != nil || relpath == "." {
				return err
			}
			targetPath := path.Join(
				projConfig.Name,
				relpath,
			)
			fileInfo, err := info.Info()
			if err != nil {
				return err
			}
			mode := fileInfo.Mode() | WRITEMODEMASK // Add write permission because embed fs is read-only
			// dir
			if info.IsDir() {
				projDirs = append(projDirs, _projDir{
					dstPath: targetPath,
					mode:    mode,
				})
				return nil
			}

			// file
			if strings.Contains(targetPath, RAW_TMPL_DIR) { // enttmpl
				projRawTmpl[filePath] = targetPath
				return nil
			}
			b, err := tmplFS.ReadFile(filePath)
			if err != nil {
				return err
			}
			projTemplates = append(projTemplates, _projTemplate{
				content: b,
				dstPath: strings.TrimSuffix(targetPath, TMPLSUFFIX),
				mode:    mode,
			})
			return nil
		},
	); err != nil {
		return err
	}
	return nil
}

func renderTemplates() {
	// dir
	for _, dir := range projDirs {
		if projConfig.SkipAsyncTask && strings.Contains(dir.dstPath, "asynctask") {
			continue
		}
		check(os.MkdirAll(dir.dstPath, dir.mode))
	}

	// file
	gobayTmpl := template.New("gobay")
	gobayTmpl.Funcs(tmplFuncs)
	for _, f := range projTemplates {
		tmpl := template.Must(gobayTmpl.Parse(string(f.content)))
		b := bytes.NewBuffer(nil)
		if err := tmpl.Execute(b, projConfig); err != nil {
			log.Fatalln(err)
		}
		// empty file
		if b.Len() <= 1 {
			continue
		}
		if err := ioutil.WriteFile(f.dstPath, b.Bytes(), f.mode); err != nil {
			log.Fatalln(err)
		}
	}
}

// copyTmplFiles copys .tpml file(like ent templates).
func copyTmplFiles() {
	for sourcePath, targetPath := range projRawTmpl {
		file, err := tmplFS.Open(sourcePath)
		if err != nil {
			panic(err)
		}
		info, err := file.Stat()
		if err != nil {
			panic(err)
		}
		b := make([]byte, info.Size())
		if _, err := file.Read(b); err != nil {
			panic(err)
		}
		check(ioutil.WriteFile(targetPath, b, FILEMODE))
	}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
