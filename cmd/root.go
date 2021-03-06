/*
Copyright © 2021 Nathan Leniz <terakilobyte@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

// TODO: refactor this into component/pipeline architecture

package cmd

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/cheggaaa/pb/v3"
	"github.com/spf13/cobra"
	"github.com/terakilobyte/checker/internal/collectors"
	"github.com/terakilobyte/checker/internal/parsers/intersphinx"
	"github.com/terakilobyte/checker/internal/parsers/rst"
	"github.com/terakilobyte/checker/internal/sources"
	"github.com/terakilobyte/checker/internal/utils"
)

var (
	path     string
	refs     bool
	docs     bool
	changes  []string
	progress bool
	workers  int
	throttle int
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "checker",
	Version: "0.1.5",
	Short:   "Checks links, and optionally :ref:s, :doc:s, and other :role:s in a docs project.",
	Long: `Checker is a tool for checking links in a docs project.
It will check refs against locally found refs and those found in intersphinx targets,
and checks roles against the latest RELEASE of rstspec.toml. Once they are validated,
all links are checked for validity.

This is mostly intended to be run on changed files only, as checking all of the links in a project
can be very time consuming.

From a git branch, run the following:

git diff --name-only HEAD master | tr "\n" "," | xargs checker -p --path . --changes

This is (nearly) the same command that should be run in CI (just omit the -p flag).
`,
	Run: func(cmd *cobra.Command, args []string) {

		if val, ok := os.LookupEnv("CHECKER_WORKERS"); ok {
			v, err := strconv.Atoi(val)
			if err != nil {
				log.Panicf("couldn't convert %s to an int: %v", val, err)
			}
			workers = v
		}

		if val, ok := os.LookupEnv("CHECKER_THROTTLE"); ok {
			v, err := strconv.Atoi(val)
			if err != nil {
				log.Panicf("couldn't convert %s to an int: %v", val, err)
			}
			throttle = v
		}

		diagnostics := make([]string, 0)
		diags := make(chan string)
		go func() {
			for d := range diags {
				diagnostics = append(diagnostics, d)
			}
		}()

		type intersphinxResult struct {
			domain string
			file   []byte
		}

		basepath, err := filepath.Abs(path)
		checkErr(err)
		snootyToml := utils.GetLocalFile(filepath.Join(basepath, "snooty.toml"))
		projectSnooty, err := sources.NewTomlConfig(snootyToml)
		checkErr(err)
		intersphinxes := make([]intersphinx.SphinxMap, len(projectSnooty.Intersphinx))
		var wgSetup sync.WaitGroup
		ixs := make(chan intersphinxResult, len(projectSnooty.Intersphinx))
		for _, intersphinx := range projectSnooty.Intersphinx {
			wgSetup.Add(1)
			go func(phx string) {
				domain := strings.Split(phx, "objects.inv")[0]
				file := utils.GetNetworkFile(phx)
				ixs <- intersphinxResult{domain: domain, file: file}
			}(intersphinx)
		}
		go func() {
			for res := range ixs {
				intersphinxes = append(intersphinxes, intersphinx.Intersphinx(res.file, res.domain))
				wgSetup.Done()
			}
		}()
		wgSetup.Wait()
		close(ixs)
		sphinxMap := intersphinx.JoinSphinxes(intersphinxes)
		files := collectors.GatherFiles(basepath)

		allShared := collectors.GatherSharedIncludes(files)

		sharedRefs := make(collectors.RstRoleMap)
		sharedLocals := make(collectors.RefTargetMap)

		for _, share := range allShared {
			sharedFile := utils.GetNetworkFile(projectSnooty.SharedPath + share.Path)
			sharedRefs.Union(collectors.GatherSharedRefs(sharedFile, *projectSnooty))
			sharedLocals.Union(collectors.GatherSharedLocalRefs(sharedFile, *projectSnooty))
		}

		allConstants := collectors.GatherConstants(files)
		allRoleTargets := collectors.GatherRoles(files)
		allHTTPLinks := collectors.GatherHTTPLinks(files)
		allLocalRefs := collectors.GatherLocalRefs(files).SSLToTLS()

		allRoleTargets.Union(sharedRefs)
		allLocalRefs.Union(sharedLocals)

		allRoleTargets = allRoleTargets.ConvertConstants(projectSnooty)

		for con, filename := range allConstants {
			if _, ok := projectSnooty.Constants[con.Name]; !ok {
				diags <- fmt.Sprintf("%s is not defined in config", con)
			}
			testCon := rst.RstConstant{Name: con.Name, Target: projectSnooty.Constants[filename] + con.Name}
			if testCon.IsHTTPLink() {
				allHTTPLinks[rst.RstHTTPLink(testCon.Target)] = filename
			}
		}

		checkedUrls := sync.Map{}
		workStack := make([]func(), 0)
		rstSpecRoles := sources.NewRoleMap(utils.GetNetworkFile(utils.GetLatestSnootyParserTag()))

		if len(changes) == 0 {
			changes = files
		}

		for role, filename := range allRoleTargets {

			if !contains(changes, strings.TrimPrefix(filename, "/")) {
				continue
			}

			switch role.Name {
			case "guilabel":
				break
			case "ref":
				if refs {
					if _, ok := sphinxMap[role.Target]; !ok {
						if _, ok := allLocalRefs.Get(&role); !ok {
							diags <- fmt.Sprintf("in %s: %+v is not a valid ref", filename, role)
						}
					}
					break
				}
			case "doc":
				if docs {
					if !contains(files, filename) {
						diags <- fmt.Sprintf("in %s: %s is not a valid file found in this docset", filename, role)
					}
					break
				}

			case "py:meth": // this is a fancy magic ref
				if refs {
					if _, ok := sphinxMap[role.Target]; !ok {
						if _, ok := allLocalRefs.Get(&role); !ok {
							diags <- fmt.Sprintf("in %s: %+v is not a valid ref", filename, role)
						}
					}
					break
				}
			case "py:class": // this is a fancy magic ref
				if refs {
					if _, ok := sphinxMap[role.Target]; !ok {
						if _, ok := allLocalRefs.Get(&role); !ok {
							diags <- fmt.Sprintf("in %s: %+v is not a valid ref", filename, role)
						}
					}
					break
				}
			default:
				if _, ok := rstSpecRoles.Roles[role.Name]; !ok {
					if _, ok := rstSpecRoles.RawRoles[role.Name]; !ok {
						if _, ok := rstSpecRoles.RstObjects[role.Name]; !ok {
							diags <- fmt.Sprintf("in %s: %s is not a valid role", filename, role)
						}
					}
					break
				}
				workFunc := func(role rst.RstRole, filename string) func() {
					url := fmt.Sprintf(rstSpecRoles.Roles[role.Name], role.Target)
					if _, ok := checkedUrls.Load(url); !ok {
						return func() {
							checkedUrls.Store(url, true)
							if resp, ok := utils.IsReachable(url); !ok {
								errmsg := fmt.Sprintf("in %s: interpeted url %s from  %+v was not valid. Got response %s", filename, url, role, resp)
								diags <- errmsg
							}
						}
					} else {
						return func() {}

					}
				}
				workStack = append(workStack, workFunc(role, filename))
			}
		}

		for link, filename := range allHTTPLinks {

			if !contains(changes, strings.TrimPrefix(filename, "/")) {
				continue
			}
			workFunc := func(link rst.RstHTTPLink, filename string) func() {
				if _, ok := checkedUrls.Load(link); !ok {
					return func() {
						checkedUrls.Store(link, true)
						if resp, ok := utils.IsReachable(string(link)); !ok {
							errmsg := fmt.Sprintf("in %s: %s is not a valid http link. Got response %s", filename, link, resp)
							diags <- errmsg
						}
					}
				} else {
					return func() {}
				}
			}

			workStack = append(workStack, workFunc(link, filename))
		}

		jobChannel := make(chan func())
		doneChannel := make(chan struct{})

		var wgValidate sync.WaitGroup
		wgValidate.Add(workers)
		for i := 0; i < workers; i++ {
			go worker(&wgValidate, jobChannel, doneChannel)
		}

		bar := pb.StartNew(len(workStack)).SetMaxWidth(120)
		if progress {
			bar.SetWriter(os.Stdout)
		} else {
			bar.SetWriter(ioutil.Discard)
		}
		go func() {
			for range doneChannel {
				bar.Increment()
			}
		}()

		for _, f := range workStack {
			jobChannel <- f
		}

		close(jobChannel)
		wgValidate.Wait()
		bar.Finish()
		for _, msg := range diagnostics {
			log.Error(msg)
		}

		if len(diagnostics) > 0 {
			log.Fatal(len(diagnostics), " errors found.\n")
		} else {
			log.Info("No errors found.\n")
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}

}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.SetVersionTemplate("checker {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&path, "path", ".", "path to the project")
	rootCmd.PersistentFlags().BoolVarP(&refs, "refs", "r", false, "check :refs:")
	rootCmd.PersistentFlags().BoolVarP(&docs, "docs", "d", false, "check :docs:")
	rootCmd.PersistentFlags().StringSliceVar(&changes, "changes", []string{}, "The list of files to check")
	rootCmd.PersistentFlags().BoolVarP(&progress, "progress", "p", false, "show progress bar")
	rootCmd.PersistentFlags().IntVarP(&workers, "workers", "w", 10, "The number of workers to spawn to do work.")
	rootCmd.PersistentFlags().IntVarP(&throttle, "throttle", "t", 10, "The throttle factor. Each worker will process at most (1e9 / (throttle / workers)) jobs per second.")
}

func checkErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if strings.Contains(a, e) {
			return true
		}
	}
	return false
}

func worker(wg *sync.WaitGroup, jobChannel <-chan func(), doneChannel chan<- struct{}) {
	defer wg.Done()
	lastExecutionTime := time.Now()
	minimumTimeBetweenEachExecution := time.Duration(math.Ceil(1e9 / (float64(throttle) / float64(workers))))
	for job := range jobChannel {
		timeUntilNextExecution := -(time.Since(lastExecutionTime) - minimumTimeBetweenEachExecution)
		if timeUntilNextExecution > 0 {
			time.Sleep(timeUntilNextExecution)
		}
		lastExecutionTime = time.Now()
		job()
		doneChannel <- struct{}{}
	}
}
