package main

import (
	"context"
	"os"

	"github.com/1password/onepassword-sdk-go"
)

func main() {
    token := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")

    client, err := onepassword.NewClient(
                context.TODO(),
                onepassword.WithServiceAccountToken(token),
                // TODO: Set the following to your own integration name and version.
                onepassword.WithIntegrationInfo("sigil", "v1.0.0"),
    )
    if err != nil {
	// handle err
    }
    secret, err := client.Secrets().Resolve(context.TODO(),"op://manahan.io/manahan-agent-app/ookaq2flae4zhrzmhrhtwzsg7m")
    if err != nil {
        // handle err
    }

    println(secret)
}
