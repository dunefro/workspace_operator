# workspace-operator
Workspace operator is a kubernetes controller that is used to create kind `workspace` which is responsible for managing `namespaces`.

Sample `workspace.yaml`
```yaml
apiVersion: environment.tf.operator.com/v1alpha1
kind: Workspace
metadata:
  name: notepad
spec:
  labels:
    deployment: "minikube"
    environment: "test"
    owner: "truefoundry"
  annotations:
    task.crd: "vedant"
  name: "test"
  resources:
    memory: "256Mi"
    cpu: "800m"
    disk: "10Gi"
  users:
    admin: "userAdmin"
    editor: "user2"
    viewer: "user3"
```
This object will create a workspace in your kubernetes with the name of `notepad` alongwith the following resources
1. `Namespace` with the name `test`
2. `ResourceQuota` with memory, cpu and disk (requests) restrictions
3. Three roles
    - Admin - `<Namespace>-admin` 
        ```yaml
        - apiGroups:
          - ""
          resources:
          - '*'
          verbs:
          - get
          - list
          - watch
          - create
          - update
          - patch
          - delete
        ``` 
    - Editor - `<Namespace>-editor`
        ```yaml
        - apiGroups:
          - ""
          resources:
          - '*'
          verbs:
          - get
          - list
          - watch
          - create
          - update
          - patch
        ```
    - Viewer - `<Namespace>-viewer` 
        ```yaml
        - apiGroups:
          - ""
          resources:
          - '*'
          verbs:
          - get
          - list
          - watch
        ```
4. Three Rolebindings attached to respective user
    - Admin - `<Namespace>-admin-rb`
    - Editor - `<Namespace>-editor-rb`
    - Viewer - `<Namespace>-viewer-rb`

## Assumptions taken
1. When the workspace controller will be bootstrapped all existig namespaces will not be governed by `workspace` because they are created outside of the `workspace` custom resource. The is done because when we run a `pod` in kubernetes it is an independent resource and deployment controller doesn't create a `deployment` just because a `pod` is existing rather it creates a `deployment` only when a custom resource of `deployment` is created so it is not necessary for a `deployment` to exist if `pod` is existing. Similarly a `namespace` can be independent of the workspace and (ideally) can exist without existence of `workspace.
2. Similarly for the above reason if a `namespace` is deleted `workspace` should (ideally) not get deleted because it is the responsibilty of the controller to maintain the state of the `workspace`. For e.g. If deployment creates a `pod` and we delete that `pod` then deployment creates the `pod` again and doesn't get deleted itself so if `namespace` is deleted then `workspace` will not get deleted and controller will rather create the `namespace` again to maitain the state of the `workspace`.
2. If we update the `spec.name` of the Custom Resource then two namespaces will be created.
3. Support for only single user in rolebindings.

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) or [MINIKUBE](https://minikube.sigs.k8s.io/docs/) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Create the controller
    ```bash
    $ kubectl apply -f install.yaml
    namespace/workspace-operator-system created
    customresourcedefinition.apiextensions.k8s.io/workspaces.environment.tf.operator.com created
    serviceaccount/workspace-operator-controller-manager created
    role.rbac.authorization.k8s.io/workspace-operator-leader-election-role created
    clusterrole.rbac.authorization.k8s.io/workspace-operator-manager-role created
    clusterrole.rbac.authorization.k8s.io/workspace-operator-metrics-reader created
    clusterrole.rbac.authorization.k8s.io/workspace-operator-namespace-clusterrole created
    clusterrole.rbac.authorization.k8s.io/workspace-operator-proxy-role created
    rolebinding.rbac.authorization.k8s.io/workspace-operator-leader-election-rolebinding created
    clusterrolebinding.rbac.authorization.k8s.io/workspace-operator-manager-rolebinding created
    clusterrolebinding.rbac.authorization.k8s.io/workspace-operator-namespace-cluster-rolebinding created
    clusterrolebinding.rbac.authorization.k8s.io/workspace-operator-proxy-rolebinding created
    service/workspace-operator-controller-manager-metrics-service created
    deployment.apps/workspace-operator-controller-manager created
    ```
2. Get the workspace resource
    ```
    $ kubectl get workspaces.environment.tf.operator.com 
    No resources found
    ```
3. Create the kind `Workspace`
    ```
    $ kubectl apply -f workspace.yaml 
    workspace.environment.tf.operator.com/test1 created
    ```
4. Check if the namespace is created
    ```
    $ kubectl get ns --selector deployment="minikube"
    NAME   STATUS   AGE
    test   Active   20s
    ```
5. Check if the roles are created
    ```
    $ kubectl -n test get roles,rolebindings --selector deployment="minikube"
    NAME                                         CREATED AT
    role.rbac.authorization.k8s.io/test-admin    2023-01-25T17:46:37Z
    role.rbac.authorization.k8s.io/test-editor   2023-01-25T17:46:40Z
    role.rbac.authorization.k8s.io/test-viewer   2023-01-25T17:46:43Z

    NAME                                                   ROLE               AGE
    rolebinding.rbac.authorization.k8s.io/test-admin-rb    Role/test-admin    42s
    rolebinding.rbac.authorization.k8s.io/test-editor-rb   Role/test-editor   39s
    rolebinding.rbac.authorization.k8s.io/test-viewer-rb   Role/test-viewer   36s
    ```
### Uninstall CRDs and controller
To delete the CRDs from the cluster:

```bash
$ kubectl delete -f install.yaml
```

## Contributing
There are two steps to contribute to the project.
1. The controller logic is written in [workspace_controller.go](./controllers/workspace_controller.go).
2. Build your docker image 
    ```bash
    TAG="<tag>"
    make docker-build docker-push IMG="<YOUR_IMAGE>:${TAG}"
    ```
3. Deploy the image 
    ```bash
    TAG="v0.1.0-beta.1"
    make deploy IMG="<YOUR_IMAGE>:{TAG}"
    ```

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster 

### Test It Out
1. Install the CRDs into the cluster:
    ```sh
    make install
    ```
2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):
    ```sh
    make run
    ```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

