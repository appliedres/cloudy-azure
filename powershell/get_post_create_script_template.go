package powershell

import _ "embed"

//go:embed vm_post_create_script_template.ps1
var PsScript string

func GetPostCreateScript() string {
    return PsScript
}