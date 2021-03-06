/*
 * Copyright 2020 Intel Corporation, Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governinog permissions and
 * limitations under the License.
 */

package manager

import (
    "github.com/akraino-edge-stack/icn-sdwan/central-controller/src/scc/pkg/module"
    "github.com/akraino-edge-stack/icn-sdwan/central-controller/src/scc/pkg/resource"

    "github.com/onap/multicloud-k8s/src/orchestrator/pkg/appcontext"
    rsyncclient "github.com/onap/multicloud-k8s/src/orchestrator/pkg/grpc/installappclient"
    controller "github.com/onap/multicloud-k8s/src/orchestrator/pkg/module/controller"
    "github.com/onap/multicloud-k8s/src/orchestrator/pkg/infra/rpc"
    "log"
    "fmt"
    "encoding/json"
    pkgerrors "github.com/pkg/errors"
)

var rsync_initialized = false
var provider_name = "akraino_scc"
var project_name = "akraino_scc"

// sdewan definition
type DeployResource struct {
    Action string
    Resource resource.ISdewanResource
}

type DeployResources struct {
    Resources []DeployResource
}

type ResUtil struct {
    resmap map[module.ControllerObject]*DeployResources
}

func NewResUtil() *ResUtil {
    if rsync_initialized == false {
        rsync_initialized = InitRsyncClient()
    }

    return &ResUtil{
        resmap: make(map[module.ControllerObject]*DeployResources),
    }
}

// --------------------------------------------------------------------------------------------------------------
// temp definition for rsync
type contextForCompositeApp struct {
    context            appcontext.AppContext
    ctxval             interface{}
    compositeAppHandle interface{}
}

func makeAppContextForCompositeApp(p, ca, v, rName string) (contextForCompositeApp, error) {
    // ctxval: context.rtcObj.id
    context := appcontext.AppContext{}
    ctxval, err := context.InitAppContext()
    if err != nil {
        return contextForCompositeApp{}, pkgerrors.Wrap(err, "Error creating AppContext CompositeApp")
    }
    // compositeHandle = context.rtc.cid
    // context.rtc.RtcCreate(): (1) save (cid, id) in etcd  
    compositeHandle, err := context.CreateCompositeApp()
    if err != nil {
        return contextForCompositeApp{}, pkgerrors.Wrap(err, "Error creating CompositeApp handle")
    }
    // (1) set context.rtcObj.meta (2) save (cid + meta +"/", json.Marshal(rtc.meta)) in etcd
    err = context.AddCompositeAppMeta(appcontext.CompositeAppMeta{Project: p, CompositeApp: ca, Version: v, Release: rName})
    if err != nil {
        return contextForCompositeApp{}, pkgerrors.Wrap(err, "Error Adding CompositeAppMeta")
    }

    // return CompositeAppMeta{Project: p, CompositeApp: ca, Version: v, Release: rn}
    //_, err = context.GetCompositeAppMeta()

    log.Println(":: The meta data stored in the runtime context :: ")

    // cca := contextForCompositeApp{context: appcontext.AppContext, ctxval: context.rtcObj.id, compositeAppHandle: context.rtc.cid}
    cca := contextForCompositeApp{context: context, ctxval: ctxval, compositeAppHandle: compositeHandle}

    return cca, nil
}

func addResourcesToCluster(ct appcontext.AppContext, ch interface{}, resources []DeployResource) error {

    var resOrderInstr struct {
        Resorder []string `json:"resorder"`
    }

    var resDepInstr struct {
        Resdep map[string]string `json:"resdependency"`
    }
    resdep := make(map[string]string)

    for _, resource := range resources {
        resource_name :=  resource.Resource.GetName() + "+" +  resource.Resource.GetType()
        resource_data := resource.Resource.ToYaml()
        resOrderInstr.Resorder = append(resOrderInstr.Resorder, resource_name)
        resdep[resource_name] = "go"
        // rtc.RtcAddResource("<cid>/app/app_name/cluster/clusername/", res.name, res.content)
        // -> save ("<cid>/app/app_name/cluster/clusername/resource/res.name/", res.content) in etcd
        // return ("<cid>/app/app_name/cluster/clusername/resource/res.name/"
        _, err := ct.AddResource(ch, resource_name, resource_data)
        if err != nil {
            cleanuperr := ct.DeleteCompositeApp()
            if cleanuperr != nil {
                log.Printf(":: Error Cleaning up AppContext after add resource failure ::")
            }
            return pkgerrors.Wrapf(err, "Error adding resource ::%s to AppContext", resource_name)
        }
        jresOrderInstr, _ := json.Marshal(resOrderInstr)
        resDepInstr.Resdep = resdep
        jresDepInstr, _ := json.Marshal(resDepInstr)
        // rtc.RtcAddInstruction("<cid>app/app_name/cluster/clusername/", "resource", "order", "{[res.name]}")
        // ->save ("<cid>/app/app_name/cluster/clusername/resource/instruction/order/", "{[res.name]}") in etcd
        // return "<cid>/app/app_name/cluster/clusername/resource/instruction/order/"
        _, err = ct.AddInstruction(ch, "resource", "order", string(jresOrderInstr))
        _, err = ct.AddInstruction(ch, "resource", "dependency", string(jresDepInstr))
        if err != nil {
            cleanuperr := ct.DeleteCompositeApp()
            if cleanuperr != nil {
                log.Printf(":: Error Cleaning up AppContext after add instruction failure ::")
            }
            return pkgerrors.Wrapf(err, "Error adding instruction for resource ::%s to AppContext", resource_name)
        }
    }
    return nil
}

func InitRsyncClient() bool {
    client := controller.NewControllerClient()

    vals, _ := client.GetControllers()
    found := false
    for _, v := range vals {
        if v.Metadata.Name == "rsync" {
            log.Println("Initializing RPC connection to resource synchronizer")
            rpc.UpdateRpcConn(v.Metadata.Name, v.Spec.Host, v.Spec.Port)
            found = true
            break
        }
    }
    return found
}

// --------------------------------------------------------------------------------------------------------------

func (d *ResUtil) AddResource(device module.ControllerObject, action string, resource resource.ISdewanResource) error {
    if d.resmap[device] == nil {
        d.resmap[device] = &DeployResources{Resources: []DeployResource{}}
    }

    d.resmap[device].Resources = append(d.resmap[device].Resources, DeployResource{Action: action, Resource: resource,})
    return nil
}

func (d *ResUtil) Deploy(format string) error {
    // Generate Application context
    cca, err := makeAppContextForCompositeApp(project_name, "app", "1", "1")
    context := cca.context  // appcontext.AppContext
    ctxval := cca.ctxval    // id
    compositeHandle := cca.compositeAppHandle // cid

    // create a com_app for each device
    for device, res := range d.resmap {
        var appOrderInstr struct {
        Apporder []string `json:"apporder"`
        }

        var appDepInstr struct {
            Appdep map[string]string `json:"appdependency"`
        }
        appdep := make(map[string]string)

        // Add application
        app_name := device.GetMetadata().Name + "_app"
        appOrderInstr.Apporder = append(appOrderInstr.Apporder, app_name)
        appdep[app_name] = "go"

        // rtc.RtcAddLevel(cid, "app", app_name) -> save ("<cid>app/app_name/", app_name) in etcd
        // apphandle = "<cid>app/app_name/"
        apphandle, _ := context.AddApp(compositeHandle, app_name)

        // Add cluster
        // err = addClustersToAppContext(listOfClusters, context, apphandle, resources)
        // rtc.RtcAddLevel("<cid>app/app_name/", "cluster", clustername) 
        // -> save ("<cid>app/app_name/cluster/clusername/", clustername) in etcd
        // return "<cid>app/app_name/cluster/clusername/"
        clusterhandle, _ := context.AddCluster(apphandle, provider_name+"+"+device.GetMetadata().Name)
        err = addResourcesToCluster(context, clusterhandle, res.Resources)
        jappOrderInstr, _ := json.Marshal(appOrderInstr)
        appDepInstr.Appdep = appdep
        jappDepInstr, _ := json.Marshal(appDepInstr)
        context.AddInstruction(compositeHandle, "app", "order", string(jappOrderInstr))
        context.AddInstruction(compositeHandle, "app", "dependency", string(jappDepInstr))
    }
    
    // invoke deployment prrocess
    appContextID := fmt.Sprintf("%v", ctxval)
    err = rsyncclient.InvokeInstallApp(appContextID)
    if err != nil {
        log.Println(err)
        return err
    }

    return nil
}