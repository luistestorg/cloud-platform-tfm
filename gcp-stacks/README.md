# GCP Stacks - Google Kubernetes Engine with Pulumi Micro-stacks

[![GCP](https://img.shields.io/badge/GCP-GKE-4285F4?logo=google-cloud)](https://cloud.google.com/kubernetes-engine)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.28-326CE5?logo=kubernetes)](https://kubernetes.io/)

Colección de micro-stacks de Pulumi para aprovisionar y gestionar infraestructura en Google Cloud Platform (GCP), con enfoque en Google Kubernetes Engine (GKE). Implementa Infrastructure as Code (IaC) con arquitectura modular y componentes reutilizables.

## 📋 Tabla de Contenidos

- [Arquitectura de Micro-stacks](#arquitectura-de-micro-stacks)
- [Prerrequisitos](#prerrequisitos)
- [Configuración del Proyecto](#configuración-del-proyecto)
- [Despliegue](#despliegue)
- [Acceso al Clúster](#acceso-al-clúster)
- [Operaciones Comunes](#operaciones-comunes)
- [Estructura de Micro-stacks](#estructura-de-micro-stacks)
- [Troubleshooting](#troubleshooting)
- [Mejores Prácticas](#mejores-prácticas)

## 🏗️ Arquitectura de Micro-stacks

```
gcp-stacks/
├── infra-gcp/          # Infraestructura GCP base (VPC, IAM)
├── infra-kube/         # Clúster GKE
├── monitoring/         # Observabilidad (Prometheus, Grafana, Logging)
├── networking/         # Load Balancing, Ingress
├── storage/            # Redis, MongoDB, MinIO
├── sql/                # Cloud SQL (PostgreSQL)
├── api/                # API services (opcional)
└── ci-support/         # CI/CD runners (opcional)
```

### Flujo de Dependencias

```
infra-gcp (VPC Network, Service Accounts, IAM)
    ↓
infra-kube (GKE Cluster + Node Pools)
    ↓
    ├── monitoring (Prometheus, Grafana, Cloud Logging)
    ├── networking (Ingress, Load Balancers)
    ├── storage (Redis, MongoDB, MinIO)
    ├── sql (Cloud SQL PostgreSQL)
    ├── api (API services, OAuth2)
    └── ci-support (GitHub Actions runners)
```

### Comunicación entre Stacks

Los micro-stacks se comunican mediante `StackReference` de Pulumi:

```go
// Ejemplo: monitoring/ consume outputs de infra-kube/
infraKube := pulumi.NewStackReference(ctx, 
    "organization/gcp-stack-infra-kube/dev", nil)

kubeconfig := infraKube.GetStringOutput(pulumi.String("kubeconfig"))
clusterEndpoint := infraKube.GetStringOutput(pulumi.String("endpoint"))
```

## 📦 Prerrequisitos

### Herramientas Requeridas

- **Google Cloud SDK (gcloud)** v400.0.0 o superior
- **Pulumi** v3.x o superior
- **Go** 1.22 o superior
- **kubectl** v1.28 o superior

### Instalación en macOS

```bash
# Google Cloud SDK
brew install google-cloud-sdk

# Pulumi
brew install pulumi

# Go
brew install go@1.22

# kubectl
brew install kubectl

# Herramientas auxiliares (recomendadas)
brew install kubectx k9s stern
```

### Instalación en Linux (Ubuntu/Debian)

```bash
# Google Cloud SDK
echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" | \
  sudo tee -a /etc/apt/sources.list.d/google-cloud-sdk.list
curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | \
  sudo apt-key --keyring /usr/share/keyrings/cloud.google.gpg add -
sudo apt-get update && sudo apt-get install google-cloud-sdk

# Pulumi
curl -fsSL https://get.pulumi.com | sh

# Go
sudo apt update
sudo apt install golang-1.22

# kubectl
sudo snap install kubectl --classic
```

### Configuración GCP

#### 1. Autenticación

```bash
# Login a GCP
gcloud auth login

# Configurar proyecto default
gcloud config set project YOUR_PROJECT_ID

# Verificar configuración
gcloud config list
```

#### 2. Habilitar APIs Necesarias

```bash
# APIs esenciales
gcloud services enable compute.googleapis.com
gcloud services enable container.googleapis.com
gcloud services enable sqladmin.googleapis.com
gcloud services enable cloudresourcemanager.googleapis.com
gcloud services enable servicenetworking.googleapis.com
gcloud services enable iam.googleapis.com

# Verificar APIs habilitadas
gcloud services list --enabled
```

#### 3. Application Default Credentials

```bash
# Para que Pulumi pueda autenticarse
gcloud auth application-default login

# Verificar
gcloud auth application-default print-access-token
```

#### 4. Crear Service Account (Opcional, para CI/CD)

```bash
# Crear service account
gcloud iam service-accounts create pulumi-deployer \
  --display-name="Pulumi Deployment Account"

# Asignar roles necesarios
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:pulumi-deployer@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/editor"

# Crear y descargar key
gcloud iam service-accounts keys create key.json \
  --iam-account=pulumi-deployer@YOUR_PROJECT_ID.iam.gserviceaccount.com

# IMPORTANTE: No commitear key.json
echo "key.json" >> .gitignore
```

## ⚙️ Configuración del Proyecto

### 1. Inicializar Backend

#### Opción A: Pulumi Cloud (Recomendado)

```bash
pulumi login
pulumi whoami
```

#### Opción B: Backend Local (Para desarrollo)

```bash
export PULUMI_BACKEND_URL="file://${HOME}/.pulumi/local-gcp"
pulumi login

# O usar script helper
source ../pulumi-local.env
```

### 2. Crear Nuevo Stack

```bash
cd gcp-stacks

# Usar script para crear stack desde template
./pulumi-cli.sh bootstrap

# Esto crea Pulumi.<stack>.yaml en cada directorio de micro-stack
```

### 3. Configurar Variables

Editar `Pulumi.<stack-name>.yaml`:

```yaml
config:
  gcp:project: your-gcp-project-id
  gcp:region: us-central1
  gcp:zone: us-central1-a  # Opcional, para recursos zonales
  
  gcp-stack:
    clusterName: my-gke-cluster
    vpcName: my-vpc
    kubeVersion: "1.28"
    env: dev
    
    # Node pool configuration
    nodePool:
      machineType: e2-medium
      minNodeCount: 1
      maxNodeCount: 10
      initialNodeCount: 3
      diskSizeGb: 50
      preemptible: false  # true para ahorro de costos
    
  sql:
    instanceTier: db-f1-micro  # Para dev, db-custom-X-Y para prod
    diskSize: 20
    backupEnabled: true
    highAvailability: false
    
  monitoring:
    grafanaEnabled: true
    grafanaDomain: grafana.example.com
    elasticsearchEnabled: false  # true para logging avanzado
    
  storage:
    redisEnabled: true
    mongodbEnabled: true
    minioEnabled: false
```

### 4. Configurar Secrets

```bash
# Secrets requeridos para diferentes stacks

# Para monitoring/ (si grafana habilitado)
export OAUTH2_CLIENT_SECRET="your-oauth-secret"
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/..."

# Para api/ (si habilitado)
export AWS_ACCESS_KEY_ID="..."      # Para ECR access
export AWS_SECRET_ACCESS_KEY="..."

# Para ci-support/ (si habilitado)
export GITHUB_APP_ID="123456"
export GITHUB_APP_INSTALLATION_ID="78910"
export GITHUB_APP_PRIVATE_KEY="$(cat github-app-key.pem)"

# O configurar como Pulumi secrets
pulumi config set --secret monitoring:oauth2ClientSecret "${OAUTH2_CLIENT_SECRET}" -C monitoring
pulumi config set --secret monitoring:slackWebhookUrl "${SLACK_WEBHOOK_URL}" -C monitoring
```

## 🚀 Despliegue

### Preview Local (Sin crear recursos)

```bash
# Preview con backend local
./pulumi-cli.sh preview -s <stack-name> --local

# Preview específico de un micro-stack
./pulumi-cli.sh preview -s <stack-name> --sub-stack-dir infra-gcp --local
```

### Despliegue Completo

```bash
# Variables de entorno requeridas (ajustar según stacks a desplegar)
export OAUTH2_CLIENT_SECRET="..."
export SLACK_WEBHOOK_URL="..."

# Inicializar todos los micro-stacks
./pulumi-cli.sh init -s <stack-name>
```

**Orden de despliegue automático:**
1. **Stack principal** (VPC base) - ~2 min
2. **infra-gcp** (Networking GCP) - ~3 min
3. **infra-kube** (GKE cluster) - ~8 min
4. **sql** (Cloud SQL) - ~5 min
5. **monitoring** (Observability) - ~3 min
6. **networking** (Load Balancing) - ~2 min
7. **storage** (Databases) - ~4 min
8. **api** (API services, opcional) - ~2 min
9. **ci-support** (CI/CD, opcional) - ~2 min

**Tiempo total estimado: ~12-15 minutos** (sin sql/api/ci-support)

### Actualización

```bash
# Actualizar todos los stacks
./pulumi-cli.sh up -s <stack-name>

# Actualizar un micro-stack específico
./pulumi-cli.sh up -s <stack-name> --sub-stack-dir sql

# Con refresh previo (recomendado)
./pulumi-cli.sh up -s <stack-name> -r
```

## 🔐 Acceso al Clúster

### Configurar kubectl

```bash
# El script lo hace automáticamente, o ejecuta:
gcloud container clusters get-credentials <cluster-name> \
  --region <region> \
  --project <project-id>

# Verificar
kubectl cluster-info
kubectl get nodes
```

**Output esperado:**
```
NAME                                      STATUS   ROLES    AGE   VERSION
gke-my-cluster-default-pool-a1b2c3d4-xyz Ready    <none>   5m    v1.28.3-gke.1098
gke-my-cluster-default-pool-e5f6g7h8-abc Ready    <none>   5m    v1.28.3-gke.1098
gke-my-cluster-default-pool-i9j0k1l2-def Ready    <none>   5m    v1.28.3-gke.1098
```

### Contextos de kubectl

```bash
# Listar contextos
kubectx

# Cambiar contexto
kubectx gke_your-project_us-central1_my-cluster

# Alias más corto
kubectx gke-dev=gke_your-project_us-central1_my-cluster
kubectx gke-dev

# Cambiar namespace por defecto
kubens monitoring
```

## 🛠️ Operaciones Comunes

### Ver Estado

```bash
# Estado del stack principal
pulumi stack -s <stack-name>

# Estado de un micro-stack
pulumi stack -s <stack-name> -C infra-kube

# Ver outputs
pulumi stack output -s <stack-name>

# Exportar kubeconfig
pulumi stack output -s <stack-name> kubeconfig > kubeconfig.yaml
```

### Refresh

```bash
# Sincronizar estado con GCP
./pulumi-cli.sh refresh -s <stack-name>

# Refresh de sub-stack específico
pulumi refresh -s <stack-name> -C sql
```

### Secrets

```bash
# Configurar secret para un micro-stack
pulumi config set --secret api:apiKey "secret-value" -s <stack-name> -C api

# Ver secrets (encriptados)
pulumi config -s <stack-name> -C api

# Ver valor de secret (requiere permisos)
pulumi config get api:apiKey -s <stack-name> -C api
```

### Destruir

```bash
# CUIDADO: Eliminará todos los recursos
./pulumi-cli.sh destroy -s <stack-name>

# El script pedirá confirmación
> Destroy operation cannot be undone!
> Please confirm that this is what you'd like to do by typing 'dev':
dev
```

## 📂 Estructura de Micro-stacks

### infra-gcp/

**Propósito**: Infraestructura GCP base

**Recursos**:
- VPC network con auto-created subnets
- Firewall rules
- Service accounts para GKE
- IAM bindings y roles
- Cloud Router y Cloud NAT (para private nodes)

**Configuración**:
```yaml
config:
  infra-gcp:
    vpcName: main-vpc
    autoCreateSubnetworks: true
    enablePrivateNodes: true
    enableCloudNat: true
```

**Outputs**:
- `vpcName`: Nombre de la VPC
- `vpcSelfLink`: Self-link de la VPC
- `serviceAccountEmail`: Email del service account para GKE

**Tiempo**: ~3 minutos

---

### infra-kube/

**Propósito**: Clúster GKE y node pools

**Recursos**:
- GKE cluster (regional o zonal)
- Node pools con autoscaling
- Workload Identity habilitado
- GKE add-ons (HTTP load balancing, HPA)
- Network policies
- Binary authorization (opcional)

**Configuración**:
```yaml
config:
  infra-kube:
    kubeVersion: "1.28"
    releaseChannel: STABLE  # RAPID, REGULAR, o STABLE
    
    nodePool:
      machineType: e2-medium
      minNodeCount: 1
      maxNodeCount: 10
      diskSizeGb: 50
      preemptible: false
      
    enableWorkloadIdentity: true
    enableNetworkPolicy: true
    enableBinaryAuthorization: false
```

**Outputs**:
- `clusterName`: Nombre del cluster
- `endpoint`: Endpoint del cluster
- `kubeconfig`: Configuración kubectl
- `clusterCaCertificate`: Certificado CA

**Tiempo**: ~8-10 minutos

---

### monitoring/

**Propósito**: Observabilidad y logging

**Recursos**:
- Prometheus & Grafana stack
- Elasticsearch & Kibana (opcional)
- Fluentd para log aggregation
- OAuth2 Proxy para autenticación
- Alertmanager con Slack integration
- Cloud Monitoring dashboards

**Configuración**:
```yaml
config:
  monitoring:
    grafanaEnabled: true
    grafanaDomain: grafana.example.com
    grafanaAdminPassword: "change-me"
    
    elasticsearchEnabled: false
    elasticsearchStorageSize: "50Gi"
    
    prometheusRetention: "30d"
    prometheusStorageSize: "50Gi"
    
    slackWebhookUrl: "https://hooks.slack.com/..."
```

**Acceso a UIs**:

```bash
# Grafana
kubectl port-forward -n monitoring svc/grafana 3000:80
# http://localhost:3000

# Prometheus
kubectl port-forward -n monitoring svc/prometheus-server 9090:80

# Kibana (si habilitado)
kubectl port-forward -n logging svc/kibana 5601:5601
```

**Tiempo**: ~3-4 minutos

---

### networking/

**Propósito**: Load balancing e ingress

**Recursos**:
- Ingress NGINX controller
- Google Cloud Load Balancer integration
- Cert-manager (Let's Encrypt)
- External DNS
- Cloud CDN (opcional)

**Configuración**:
```yaml
config:
  networking:
    ingressClass: nginx
    enableTLS: true
    certManagerEmail: admin@example.com
    
    enableExternalDns: true
    domainFilter: example.com
    
    enableCloudCDN: false
    loadBalancerType: external  # o internal
```

**Ejemplo de Ingress**:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - app.example.com
    secretName: app-tls
  rules:
  - host: app.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: my-app
            port:
              number: 80
```

**Tiempo**: ~2-3 minutos

---

### storage/

**Propósito**: Bases de datos y almacenamiento

**Recursos**:
- Redis cluster (Bitnami chart)
- MongoDB replica set
- MinIO (S3-compatible storage)
- Persistent volumes
- Backup CronJobs

**Configuración**:
```yaml
config:
  storage:
    redisEnabled: true
    redisClusterSize: 3
    redisMemory: "2Gi"
    
    mongodbEnabled: true
    mongodbReplicas: 3
    mongodbStorageSize: "20Gi"
    
    minioEnabled: false
    minioStorageSize: "50Gi"
```

**Acceso**:
```bash
# Redis
kubectl port-forward -n storage svc/redis-master 6379:6379
redis-cli -h localhost

# MongoDB
kubectl port-forward -n storage svc/mongodb 27017:27017
mongo mongodb://localhost:27017
```

**Tiempo**: ~4-5 minutos

---

### sql/

**Propósito**: Cloud SQL PostgreSQL gestionado

**Recursos**:
- Cloud SQL PostgreSQL instance
- Databases y users
- Private IP para conexión desde GKE
- Automated backups
- Kubernetes secrets con credenciales
- Cloud SQL Proxy sidecar

**Configuración**:
```yaml
config:
  sql:
    instanceTier: db-custom-2-7680  # vCPUs-Memory(MB)
    diskSize: 20
    diskType: PD_SSD  # o PD_HDD
    
    backupEnabled: true
    backupStartTime: "03:00"
    
    highAvailability: false
    region: us-central1
    
    databases:
      - name: app_db
        charset: UTF8
    
    users:
      - name: app_user
        # password generado automáticamente
```

**Acceso desde pods**:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
  - name: app
    image: my-app:latest
    env:
    - name: DB_HOST
      value: "127.0.0.1"
    - name: DB_PORT
      value: "5432"
    - name: DB_NAME
      valueFrom:
        secretKeyRef:
          name: cloudsql-db-credentials
          key: database
    - name: DB_USER
      valueFrom:
        secretKeyRef:
          name: cloudsql-db-credentials
          key: username
    - name: DB_PASSWORD
      valueFrom:
        secretKeyRef:
          name: cloudsql-db-credentials
          key: password
  
  # Cloud SQL Proxy sidecar
  - name: cloud-sql-proxy
    image: gcr.io/cloudsql-docker/gce-proxy:latest
    command:
      - "/cloud_sql_proxy"
      - "-instances=PROJECT:REGION:INSTANCE=tcp:5432"
    securityContext:
      runAsNonRoot: true
```

**Tiempo**: ~5-8 minutos

---

### api/ (Opcional)

**Propósito**: API services y endpoints

**Recursos**:
- API deployments
- Services y Ingress
- Redis para cache
- OAuth2 authentication
- Rate limiting

**Configuración**:
```yaml
config:
  api:
    replicas: 3
    domain: api.example.com
    cacheEnabled: true
    oauth2Enabled: true
```

**Tiempo**: ~2-3 minutos

---

### ci-support/ (Opcional)

**Propósito**: CI/CD infrastructure

**Recursos**:
- GitHub Actions self-hosted runners
- Artifact registry
- Build agents
- CI/CD credentials

**Configuración**:
```yaml
config:
  ci-support:
    githubAppId: "123456"
    runnerReplicas: 2
    runnerLabels: ["gcp", "gke"]
```

**Tiempo**: ~2-3 minutos

---

## 🔧 Troubleshooting

### Errores Comunes

#### 1. Error de Permisos

**Síntoma**:
```
Error 403: Permission denied on resource project
```

**Solución**:
```bash
# Verificar permisos del proyecto
gcloud projects get-iam-policy YOUR_PROJECT_ID

# Re-autenticar
gcloud auth application-default login

# Verificar service account (si usas uno)
gcloud iam service-accounts get-iam-policy \
  SERVICE_ACCOUNT_EMAIL
```

#### 2. APIs No Habilitadas

**Síntoma**:
```
Error 403: compute.googleapis.com API has not been used
```

**Solución**:
```bash
# Habilitar API necesaria
gcloud services enable compute.googleapis.com

# Ver todas las APIs requeridas
gcloud services list --available | grep -E "(compute|container|sql)"

# Habilitar todas a la vez
gcloud services enable compute.googleapis.com \
  container.googleapis.com \
  sqladmin.googleapis.com
```

#### 3. Cluster Unreachable

**Síntoma**:
```
Unable to connect to the server: dial tcp: lookup XXX
```

**Solución**:
```bash
# Re-obtener credenciales
gcloud container clusters get-credentials <cluster-name> \
  --region <region> \
  --project <project-id>

# Verificar contexto
kubectl config current-context

# Verificar firewall si es private cluster
gcloud compute firewall-rules list
```

#### 4. Estado Desincronizado

**Síntoma**:
```
error: resource already exists but wasn't in the state
```

**Solución**:
```bash
# Refresh completo
./pulumi-cli.sh refresh -s <stack-name>

# Si persiste, exportar e importar estado
pulumi stack export -s <stack-name> > state-backup.json
# Revisar y editar si necesario
pulumi stack import -s <stack-name> < state-backup.json
```

#### 5. Cuota Excedida

**Síntoma**:
```
Error 429: Quota exceeded for quota metric 'cpus' and limit 'CPUS-per-project-REGION'
```

**Solución**:
```bash
# Ver cuotas actuales
gcloud compute project-info describe --project=YOUR_PROJECT_ID

# Solicitar aumento de cuota
# https://console.cloud.google.com/iam-admin/quotas

# Alternativa: usar región diferente o reducir recursos
```

### Logs y Debugging

```bash
# Ver logs de deployment
pulumi logs -s <stack-name>

# Logs de GKE
gcloud container clusters get-credentials <cluster> --region <region>
kubectl logs -n <namespace> <pod> --tail=100

# Cloud Logging
gcloud logging read "resource.type=k8s_cluster" --limit 50

# Eventos del cluster
kubectl get events --all-namespaces --sort-by='.lastTimestamp'
```

## ✅ Mejores Prácticas

### 1. Preview Siempre

```bash
./pulumi-cli.sh preview -s <stack-name> --local
```

### 2. Variables de Entorno

Crear archivo `.env` (NO commitear):

```bash
export OAUTH2_CLIENT_SECRET="..."
export SLACK_WEBHOOK_URL="..."
export AWS_ACCESS_KEY_ID="..."
export AWS_SECRET_ACCESS_KEY="..."
export GITHUB_APP_PRIVATE_KEY="..."

# Source antes de deployar
source .env
```

### 3. Respaldos

```bash
# Respaldar estado antes de cambios mayores
pulumi stack export -s <stack-name> > backup-$(date +%Y%m%d).json

# Respaldar todos los sub-stacks
for stack in infra-gcp sql monitoring; do
  pulumi stack export -s <stack-name> -C $stack > backup-$stack-$(date +%Y%m%d).json
done
```

### 4. Cost Management

```yaml
# Usar preemptible nodes para dev
nodePool:
  preemptible: true  # Hasta 80% de ahorro
  
# Autoscaling agresivo
nodePool:
  minNodeCount: 1
  maxNodeCount: 10
  
# Tier pequeño para dev
sql:
  instanceTier: db-f1-micro  # ~$7/mes vs db-custom-2-7680 ~$120/mes
```

**Monitorear costos**:
```bash
# Ver costos del proyecto
gcloud billing accounts list
gcloud beta billing projects describe YOUR_PROJECT_ID
```

### 5. Workload Identity

GKE usa Workload Identity para acceso seguro a GCP APIs:

```bash
# Crear service account
gcloud iam service-accounts create my-app-sa

# Dar permisos necesarios
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:my-app-sa@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/storage.objectViewer"

# Vincular con Kubernetes service account
gcloud iam service-accounts add-iam-policy-binding \
  my-app-sa@YOUR_PROJECT_ID.iam.gserviceaccount.com \
  --role roles/iam.workloadIdentityUser \
  --member "serviceAccount:YOUR_PROJECT_ID.svc.id.goog[NAMESPACE/KSA_NAME]"
```

**En Kubernetes**:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-app
  annotations:
    iam.gke.io/gcp-service-account: my-app-sa@YOUR_PROJECT_ID.iam.gserviceaccount.com
```

### 6. Secrets Management

```bash
# Usar Google Secret Manager
echo -n "my-secret-value" | \
  gcloud secrets create my-secret --data-file=-

# Acceder desde pod (con Workload Identity)
gcloud secrets versions access latest --secret=my-secret
```

### 7. Actualización de go.mod

```bash
# Mantener dependencias actualizadas
./pulumi-cli.sh mod_tidy -s <stack-name>
```

## 📚 Referencias

- [Pulumi GCP Provider](https://www.pulumi.com/registry/packages/gcp/)
- [GKE Best Practices](https://cloud.google.com/kubernetes-engine/docs/best-practices)
- [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
- [Cloud SQL Best Practices](https://cloud.google.com/sql/docs/postgres/best-practices)

## 🔗 Links Útiles

- [GCP Free Tier](https://cloud.google.com/free)
- [GKE Pricing Calculator](https://cloud.google.com/products/calculator)
- [GCP Architecture Center](https://cloud.google.com/architecture)

---

**Parte del TFM**: "Implementación de una Estrategia Multicloud para el Despliegue de Aplicaciones Usando Pulumi Micro-stacks"