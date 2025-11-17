package ecrSecretUpdater

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/chialab/aws-ecr-get-login-password/ecr"
	"github.com/ncruces/go-gcp/glog"
)

const (
	ecrSecretName = "CloudEngSA_ECR"
)

func init() {
	functions.HTTP("RetrieveToken", RetrieveToken)
}

// RetrieveToken Function.
func RetrieveToken(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	project := os.Getenv("PROJECT")
	if token, err := ecr.GetToken(ctx); err != nil {
		http.Error(w, "Cannot retrieve token", http.StatusInternalServerError)
		glog.Errorln(fmt.Sprintf("Cannot retrieve token - error %s", err))
	} else {
		client, err := secretmanager.NewClient(ctx)
		if err != nil {
			http.Error(w, "Cannot connect to secret manager", http.StatusInternalServerError)
			glog.Errorln(fmt.Sprintf("Cannot connect to secret manager - error %s", err))
		}
		defer client.Close()

		secretpath := fmt.Sprintf("projects/%s/secrets/%s", project, ecrSecretName)

		req := &secretmanagerpb.AddSecretVersionRequest{
			Parent: secretpath,
			Payload: &secretmanagerpb.SecretPayload{
				Data: []byte(*token.Password),
			},
		}
		result, err := client.AddSecretVersion(ctx, req)
		if err != nil {
			http.Error(w, "Failed to add secret version", http.StatusInternalServerError)
			glog.Errorln(fmt.Sprintf("failed to add secret version: - error %s", err))
		}
		fmt.Fprintf(w, "Added secret version: %s\n", result.Name)
		glog.Infoln(fmt.Sprintf("Added secret version: %s", result.Name))

		//Disables old version
		lastVersionNumber, err := strconv.Atoi(result.Name[strings.LastIndex(result.Name, "/")+1:])
		if err != nil {
			glog.Errorln(fmt.Sprintf("String to int conversion error %s", err))
		}
		lastVersionName := fmt.Sprintf(secretpath+"/versions/%d", lastVersionNumber-1)

		reqDis := &secretmanagerpb.DisableSecretVersionRequest{
			Name: lastVersionName,
		}

		// Call the API.
		if _, err := client.DisableSecretVersion(ctx, reqDis); err != nil {
			http.Error(w, "Failed to disable secret version", http.StatusInternalServerError)
			glog.Errorln(fmt.Sprintf("Failed to disable secret version %s", err))
		} else {
			glog.Infoln(fmt.Sprintf("Disabled unused version : %s", lastVersionName))
		}

	}
}
