module github.com/nhurel/terraspec

go 1.14

require (
	github.com/hashicorp/go-hclog v0.9.2
	github.com/hashicorp/go-plugin v1.3.0
	github.com/hashicorp/go-version v1.2.0
	github.com/hashicorp/hcl/v2 v2.6.0
	github.com/hashicorp/terraform v0.13.2
	github.com/hashicorp/terraform-svchost v0.0.0-20191011084731-65d371908596
	github.com/mitchellh/cli v1.0.0
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db
	github.com/mitchellh/go-homedir v1.1.0
	github.com/zclconf/go-cty v1.5.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
)

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
