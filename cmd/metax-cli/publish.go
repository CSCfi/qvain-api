package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/CSCfi/qvain-api/internal/psql"
	"github.com/CSCfi/qvain-api/internal/shared"
	"github.com/CSCfi/qvain-api/pkg/env"
	"github.com/CSCfi/qvain-api/pkg/metax"
	"github.com/CSCfi/qvain-api/pkg/models"
	"github.com/wvh/uuid"
	uuidflag "github.com/wvh/uuid/flag"
)

type stringsFlag []string

func (s *stringsFlag) String() string {
	return ""
}

func (s *stringsFlag) Set(val string) error {
	*s = append(*s, strings.Split(val, ",")...)
	return nil
}

func runPublish(url string, args []string) error {
	flags := flag.NewFlagSet("publish", flag.ExitOnError)
	var (
		ownerUuid uuidflag.Uuid
		projects  stringsFlag
	)
	flags.Var(&ownerUuid, "owner", "owner `uuid` to check dataset ownership against")
	flags.Var(&projects, "projects", "comma-separated list of IDA projects used in the dataset")

	flags.Usage = usageFor(flags, "publish [flags] <id>")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() < 1 {
		flags.Usage()
		return fmt.Errorf("error: missing dataset id argument")
	}

	id, err := uuid.FromString(flags.Arg(0))
	if err != nil {
		return err
	}

	if ownerUuid.IsSet() {
		fmt.Println("User:", ownerUuid)
	}

	db, err := psql.NewPoolServiceFromEnv()
	if err != nil {
		return err
	}

	api := metax.NewMetaxService(
		os.Getenv("APP_METAX_API_HOST"),
		metax.WithCredentials(os.Getenv("APP_METAX_API_USER"), os.Getenv("APP_METAX_API_PASS")),
		metax.WithInsecureCertificates(env.GetBool("APP_DEV_MODE")),
	)

	owner := &models.User{
		Uid:      ownerUuid.Get(),
		Projects: projects,
	}
	vId, nId, qId, err := shared.Publish(api, db, id, owner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "type: %T\n", err)
		if apiErr, ok := err.(*metax.ApiError); ok {
			fmt.Fprintf(os.Stderr, "metax error: %s\n", apiErr.OriginalError())
		}
		if dbErr, ok := err.(*psql.DatabaseError); ok {
			fmt.Fprintf(os.Stderr, "database error: %s\n", dbErr.Error())
		}
		return err
	}

	fmt.Fprintln(os.Stderr, "success")
	fmt.Fprintln(os.Stderr, "metax identifier:", vId)
	if nId != "" {
		fmt.Fprintln(os.Stderr, "metax identifier (new version):", nId)
		fmt.Fprintln(os.Stderr, "qvain identifier (new version):", qId)
	}
	return nil
}
