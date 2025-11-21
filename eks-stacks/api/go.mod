module cloud-platform-tfm/eks-stacks/api

go 1.21

require (
	github.com/pulumi/pulumi-aws/sdk/v5 v5.42.0
	github.com/pulumi/pulumi-kubernetes/sdk/v4 v4.9.1
	github.com/pulumi/pulumi/sdk/v3 v3.112.0
	tracemachina.com/shared v0.0.0
)

replace tracemachina.com/shared => ../../shared
