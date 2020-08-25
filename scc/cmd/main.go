/*
Copyright 2020 Intel Corporation.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
    "context"
    "log"
    "math/rand"
    "net/http"
    "os"
    "os/signal"
    "time"

    "github.com/akraino-edge-stack/icn-sdwan/central-controller/src/scc/api"
    "github.com/akraino-edge-stack/icn-sdwan/central-controller/src/scc/pkg/infra/auth"
    "github.com/akraino-edge-stack/icn-sdwan/central-controller/src/scc/pkg/infra/config"
    "github.com/akraino-edge-stack/icn-sdwan/central-controller/src/scc/pkg/infra/db"
    tmpdb "github.com/onap/multicloud-k8s/src/orchestrator/pkg/infra/db"
//    contextDb "github.com/akraino-edge-stack/icn-sdwan/central-controller/src/scc/pkg/infra/contextdb"
    contextDb "github.com/onap/multicloud-k8s/src/orchestrator/pkg/infra/contextdb"
    "github.com/gorilla/handlers"
)

func main() {

    rand.Seed(time.Now().UnixNano())

    tmpdb.InitializeDatabaseConnection("scc")
    err := db.InitializeDatabaseConnection("scc")
    if err != nil {
            log.Println("Unable to initialize database connection...")
            log.Println(err)
            log.Fatalln("Exiting...")
    }

    err = contextDb.InitializeContextDatabase()
    if err != nil {
            log.Println("Unable to initialize database connection...")
            log.Println(err)
            log.Fatalln("Exiting...")
    }

    httpRouter := api.NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
    loggedRouter := handlers.LoggingHandler(os.Stdout, httpRouter)
    log.Println("Starting SDEWAN Central Controller API")

    httpServer := &http.Server{
            Handler:    loggedRouter,
            Addr:       ":" + config.GetConfiguration().ServicePort,
    }

    connectionsClose := make(chan struct{})
    go func() {
            c := make(chan os.Signal, 1)
            signal.Notify(c, os.Interrupt)
            <-c
            httpServer.Shutdown(context.Background())
            close(connectionsClose)
    }()

    tlsConfig, err := auth.GetTLSConfig("ca.cert", "server.cert", "server.key")
    if err != nil {
        log.Println("Error Getting TLS Configuration. Starting without TLS...")
        log.Fatal(httpServer.ListenAndServe())
    } else {
            httpServer.TLSConfig = tlsConfig

            err = httpServer.ListenAndServeTLS("", "")
    }
}
