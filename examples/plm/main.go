package main

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

func main() {
	var autoJSONTmpl = template.Must(template.New("auto").Parse(`{
"kind":"MultidimPodAutoscaler",
"metadata":{
	"name":"{{.Name}}",
	"namespace":"{{.Namespace}}",
	"labels":{
		"serviceweaver/app_name":"{{.AppKey}}",
		"serviceweaver/deployment_id":"{{.VersionKey}}"
	},
},
"spec":{
	"constraints":{
		"container":[{
			"name":"{{.AppContainerName}}",
			"requests":{"minAllowed":{"memory":"{{.MinMemory}}"}}
		}],
		"containerControlledResources":["memory"],
		"global":{"maxReplicas":999999,"minReplicas":{{.MinReplicas}}}
	},
	"goals":{
		"metrics":[{
			"resource":{
				"name":"cpu",
				"target":{
					"averageUtilization":80,
					"type":"Utilization"
				}
			},
			"type":"Resource"
		}]
	},
	"policy":{"updateMode":"Auto"},
	"scaleTargetRef":{
		"apiVersion":"apps/v1",
		"kind":"{{.TargetKind}}",
		"name":"{{.TargetName}}"
	}
}}`))

	var b strings.Builder
	if err := autoJSONTmpl.Execute(&b, struct {
		Name             string
		Namespace        string
		AppKey           string
		VersionKey       string
		AppNameLabel     string
		AppVersionLabel  string
		AppContainerName string
		MinMemory        string
		MinReplicas      int32
		TargetKind       string
		TargetName       string
		Metadata         map[string]interface{}
	}{
		Name:             "myname",
		Namespace:        "mynamespace",
		AppKey:           "appkey",
		VersionKey:       "versionkey",
		AppNameLabel:     "appnamelabel",
		AppVersionLabel:  "appversionlabel",
		AppContainerName: "appContainerName",
		MinMemory:        "memoryUnit",
		MinReplicas:      2,
		TargetKind:       "deployment",
		TargetName:       "aName",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Robert - can't execute template \n")
	}
	fmt.Fprintf(os.Stderr, "Robert - template: %s\n", b.String())
}
