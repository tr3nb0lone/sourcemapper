package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"

	smapper "github.com/tr3nb0lone/sourcemapper/sourcemapper"
)

func main() {
	var conf smapper.Config
	var err error

	flag.StringVar(&conf.Outdir, "output", "", "Source file output directory - REQUIRED")
	flag.StringVar(&conf.Url, "url", "", "URL or path to the Sourcemap file - cannot be used with jsurl")
	flag.StringVar(&conf.Jsurl, "jsurl", "", "URL to JavaScript file - cannot be used with url")
	help := flag.Bool("help", false, "Show help")
	flag.Parse()

	if *help || (conf.Url == "" && conf.Jsurl == "") || conf.Outdir == "" {
		flag.Usage()
		return
	}

	if conf.Jsurl != "" && conf.Url != "" {
		log.Println("[!] Both -jsurl and -url supplied")
		flag.Usage()
		return
	}

	var sm smapper.SourceMap

	// these need to just take the conf object
	if conf.Url != "" {
		if sm, err = smapper.GetSourceMap(conf.Url); err != nil {
			log.Fatal(err)
		}
	} else if conf.Jsurl != "" {
		if sm, err = smapper.GetSourceMapFromJS(conf.Jsurl); err != nil {
			log.Fatal(err)
		}
	}

	// everything below needs to go into its own function
	log.Printf("[+] Retrieved Sourcemap with version %d, containing %d entries.\n", sm.Version, len(sm.Sources))

	if len(sm.Sources) == 0 {
		log.Fatal("No sources found.")
	}

	if len(sm.SourcesContent) == 0 {
		log.Fatal("No source content found.")
	}

	if sm.Version != 3 {
		log.Println("[!] Sourcemap is not version 3. This is untested!")
	}

	if _, err := os.Stat(conf.Outdir); os.IsNotExist(err) {
		err = os.Mkdir(conf.Outdir, 0700)
		if err != nil {
			log.Fatal(err)
		}
	}

	for i, sourcePath := range sm.Sources {
		sourcePath = "/" + sourcePath // path.Clean will ignore a leading '..', must be a '/..'
		// If on windows, clean the sourcepath.
		if runtime.GOOS == "windows" {
			sourcePath = smapper.CleanWindows(sourcePath)
		}

		// Use filepath.Join. https://parsiya.net/blog/2019-03-09-path.join-considered-harmful/
		scriptPath, scriptData := filepath.Join(conf.Outdir, filepath.Clean(sourcePath)), sm.SourcesContent[i]
		err := smapper.WriteFile(scriptPath, scriptData)
		if err != nil {
			log.Printf("Error writing %s file: %s", scriptPath, err)
		}
	}

	log.Println("[+] Done")
}
