package main

import (
	"fmt"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"treeverse-lake/auth"
	"treeverse-lake/auth/model"
	"treeverse-lake/block"
	db2 "treeverse-lake/db"
	"treeverse-lake/gateway"
	"treeverse-lake/gateway/permissions"
	"treeverse-lake/index"
	"treeverse-lake/index/store"

	"github.com/dgraph-io/badger"

	log "github.com/sirupsen/logrus"
)

var (
	DefaultBlockLocation    = path.Join(home(), "tv_state", "blocks")
	DefaultMetadataLocation = path.Join(home(), "tv_state", "kv")
)

func home() string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	return u.HomeDir
}

func createCreds() {
	// init db
	db, err := badger.Open(badger.DefaultOptions(DefaultMetadataLocation))
	if err != nil {
		panic(err)
	}

	// init auth
	authService := auth.NewKVAuthService(db)
	err = authService.CreateUser(&model.User{
		Id:       "exampleuid",
		Email:    "ozkatz100@gmail.com",
		FullName: "Oz Katz",
	})
	if err != nil {
		panic(err)
	}

	err = authService.CreateRole(&model.Role{
		Id:   "examplerid",
		Name: "AdminRole",
		Policies: []*model.Policy{
			{
				Permission: permissions.PermissionManageRepos,
				Arn:        "arn:treeverse:repos:::*",
			},
			{
				Permission: permissions.PermissionReadRepo,
				Arn:        "arn:treeverse:repos:::*",
			},
			{
				Permission: permissions.PermissionWriteRepo,
				Arn:        "arn:treeverse:repos:::*",
			},
		},
	})
	if err != nil {
		panic(err)
	}

	err = authService.AssignRoleToUser("examplerid", "exampleuid")
	if err != nil {
		panic(err)
	}

	creds, err := authService.CreateUserCredentials(&model.User{Id: "exampleuid"})
	if err != nil {
		panic(err)
	}

	fmt.Printf("creds:\naccess: %s\nsecret: %s\n", creds.GetAccessKeyId(), creds.GetAccessSecretKey())
}

func Run() {
	// logger
	log.SetReportCaller(true)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.TraceLevel) // for now

	// init db
	// a quick fix for a crash on windows when a server is restarted.
	// the solution is to delete the value log (*.vlog) files from tv_state\kv directory
	//todo: check if this is safe in a running system, where objects are added and deleted.
	// will it lose data? is there a way to lose less?
	//todo: the initial allocation in windows is 2GB for each vlog file.
	// the reason is that badgerDB access the vlog as memory mapped file. on windows mmap-ed
	// files can not be extended. This is probably way more than a test deployment in windows needs.
	// need to see it this size can be configured to a smaller size (e.g. 100MB)
	if runtime.GOOS == "windows" {
		logFilesPattern := path.Join(DefaultMetadataLocation, "*.vlog")
		logFilesList, err := filepath.Glob(logFilesPattern)
		if err != nil {
			panic(err)
		}
		if logFilesList != nil {
			for _, fileName := range logFilesList {
				err := os.Remove(fileName)
				if err != nil {
					panic(err)
				}
			}
		}
	}

	db, err := badger.Open(badger.DefaultOptions(DefaultMetadataLocation))
	if err != nil {
		panic(err)
	}

	// init index
	meta := index.NewKVIndex(store.NewKVStore(db))

	// init mpu manager
	mpu := index.NewKVMultipartManager(store.NewKVStore(db))

	// init block store
	blockStore, err := block.NewLocalFSAdapter(DefaultBlockLocation)
	if err != nil {
		panic(err)
	}

	// init authentication
	authService := auth.NewKVAuthService(db)

	// init gateway server
	server := gateway.NewServer("us-east-1", meta, blockStore, authService, mpu, "0.0.0.0:8000", "s3.local:8000")
	panic(server.Listen())
}

func keys() {
	// init db
	db, err := badger.Open(badger.DefaultOptions(DefaultMetadataLocation))
	if err != nil {
		panic(err)
	}
	err = db.View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		iter := tx.NewIterator(opts)
		defer iter.Close()
		for iter.Rewind(); iter.Valid(); iter.Next() {
			item := iter.Item()
			key := item.Key()
			k := db2.KeyFromBytes(key)
			fmt.Printf("%s\n", k)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	err = db.Close()
	if err != nil {
		panic(err)
	}
}

func main() {
	switch os.Args[1] {
	case "run":
		Run()
	case "creds":
		createCreds()
	case "keys":
		keys()
	}
}
