package create

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"

	"github.com/hofstadter-io/hof/cmd/hof/flags"
	"github.com/hofstadter-io/hof/lib/gen"
)

func Create(args []string, rootflags flags.RootPflagpole, cmdflags flags.CreateFlagpole) error {

	// fmt.Println("Create:", args, cmdflags)

	// TODO, is this local or remote?
	if len(args) == 1 && !strings.HasPrefix(args[0], ".") {
		fmt.Println("Remote repositories are not supported yet")
		return nil
	}

	// Do local generators need to be moved to vendor so that
	// template lookups still work???

	genflags := flags.GenFlags
	genflags.Generator = cmdflags.Generator
	genflags.Diff3 = false
	R, err := gen.NewRuntime(args, rootflags, genflags)
	if err != nil {
		return err
	}

	if R.Verbosity > 0 {
		fmt.Println("CueDirs:", R.CueModuleRoot, R.WorkingDir)
	}

	// minimally load and extract generators
	err = R.LoadCue()
	if err != nil {
		return err
	}

	err = R.ExtractGenerators()
	if err != nil {
		return err
	}

	// handle create input / prompt
	for _, G := range R.Generators {
		err = handleGeneratorCreate(G)
		if err != nil {
			return err
		}
	}

	// full load generators
	errs := R.LoadGenerators()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println(e)
		}
		return fmt.Errorf("While loading generators")
	}

	// run generators
	errs = R.RunGenerators()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println(e)
		}
		return fmt.Errorf("While generating")
	}

	// write output
	errs = R.WriteOutput()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println(e)
		}
		return fmt.Errorf("While writing")
	}

	for _, G := range R.Generators {
		after := G.CueValue.LookupPath(cue.ParsePath("CreateMessage.After"))
		if after.Err() != nil {
			fmt.Println("error:", after.Err())
			return after.Err()
		}

		if !after.IsConcrete() || !after.Exists() {
			if R.Verbosity > 0 {
				fmt.Println("done creating")
			}
		} else {
			s, err := after.String()
			if err != nil {
				return err
			}
			fmt.Println(s)
		}
	}

	return nil
}

func handleGeneratorCreate(G *gen.Generator) error {
	before := G.CueValue.LookupPath(cue.ParsePath("CreateMessage.Before"))
	if before.Err() != nil {
		fmt.Println("error:", before.Err())
		return before.Err()
	}

	if !before.IsConcrete() || !before.Exists() {
		fmt.Printf("Creating from %q\n", G.Name)
	} else {
		s, err := before.String()
		if err != nil {
			return err
		}
		fmt.Println(s)
	}

	val := G.CueValue.LookupPath(cue.ParsePath("CreateInput"))
	if val.Err() != nil {
		fmt.Println("error:", val.Err())
		return val.Err()
	}

	if !val.IsConcrete() {
		return fmt.Errorf("Generator is missing CreateInput")
	}

	prompt := G.CueValue.LookupPath(cue.ParsePath("CreatePrompt"))
	if prompt.Err() != nil {
		fmt.Println("error:", prompt.Err())
		return prompt.Err()
	}

	if !prompt.IsConcrete() || !prompt.Exists() {
		return fmt.Errorf("Generator is missing CreatePrompt")
	}

	// fmt.Printf("%s: %v\n", G.Name, val)
	// fmt.Println(prompt)

	ans := map[string]any{}
	// TODO deal with --input flags

	// process create prompts
	// Loop through all top level fields
	iter, err := prompt.List()
	if err != nil {
		return err
	}
	for iter.Next() {
		value := iter.Value()
		Q := map[string]any{}
		err := value.Decode(&Q)
		if err != nil {
			return err
		}

		// fmt.Println("q:", Q)
		// todo, extract Name
		A, err := handleQuestion(Q)
		if err != nil {
			return err
		}

		// do we want to return a bool from handleQuestion
		// to be more explicit about this check?
		if A != nil {
			ans[Q["Name"].(string)] = A
		}
	}

	// fill CreateInput from --inputs and prompt
	G.CueValue = G.CueValue.FillPath(cue.ParsePath("CreateInput"), ans)

	// fmt.Println("Final:", G.CueValue)
	// return fmt.Errorf("intentional error")
	return nil
}

func handleQuestion(Q map[string]any) (A any, err error) {
	// fmt.Println(" -", Q["Name"], Q["Type"])

	err = fmt.Errorf("not asked yet")
	for err != nil {
		err = nil
		switch Q["Type"] {
		case "input":
			dval := ""
			if d, ok := Q["Default"]; ok {
				dval = d.(string)
			}
			prompt := &survey.Input {
				// todo, rename prompt to message in test and schema
				Message: Q["Prompt"].(string),
				Default: dval,
			}
			var a string
			err = survey.AskOne(prompt, &a)
			A = a

		case "multiline":
			dval := ""
			if d, ok := Q["Default"]; ok {
				dval = d.(string)
			}
			prompt := &survey.Multiline {
				// todo, rename prompt to message in test and schema
				Message: Q["Prompt"].(string),
				Default: dval,
			}
			var a string
			err = survey.AskOne(prompt, &a)
			A = a

		case "password":
			prompt := &survey.Password {
				// todo, rename prompt to message in test and schema
				Message: Q["Prompt"].(string),
			}
			var a string
			err = survey.AskOne(prompt, &a)
			A = a

		case "confirm":
			dval := false
			if d, ok := Q["Default"]; ok {
				dval = d.(bool)
			}
			prompt := &survey.Confirm {
				// todo, rename prompt to message in test and schema
				Message: Q["Prompt"].(string),
				Default: dval,
			}
			var a bool
			err = survey.AskOne(prompt, &a)

			if !a {
				return nil, nil
			} else {
				A = a
			}
			// possibly recurse if has own Questions
			QS, ok := Q["Questions"]
			if a && ok {
				A2 := map[string]any{}
				for _, Q2 := range QS.([]any) {
					q2 := Q2.(map[string]any)
					a2, e2 := handleQuestion(q2)
					// todo, think about if/how to handle this error
					if e2 != nil {
						return nil, e2
					}
					A2[q2["Name"].(string)] = a2
				}
				A = A2
			}

		case "select":
			opts := []string{}
			for _, o := range Q["Options"].([]any) {
				opts = append(opts, o.(string))
			}
			prompt := &survey.Select {
				// todo, rename prompt to message in test and schema
				Message: Q["Prompt"].(string),
				// todo, probably need to error handle options
				Options: opts,
				Default: Q["Default"],
			}
			var a string
			err = survey.AskOne(prompt, &a)
			A = a

		case "multiselect":
			opts := []string{}
			for _, o := range Q["Options"].([]any) {
				opts = append(opts, o.(string))
			}
			prompt := &survey.MultiSelect {
				// todo, rename prompt to message in test and schema
				Message: Q["Prompt"].(string),
				// todo, probably need to error handle options
				Options: opts,
				Default: Q["Default"],
			}
			var a []string
			err = survey.AskOne(prompt, &a)
			A = a

		default:
			// should never get here unless we have mismatch between schema and values
			panic("unknown question type: " + Q["Type"].(string))

		}

		if err != nil {
			if err == terminal.InterruptErr {
				return nil, fmt.Errorf("user interrupt")
			}
			fmt.Println("error:", err)
		} else {
			// fmt.Println("you said:", A)
			break
		}
	}

	return A, nil
}
