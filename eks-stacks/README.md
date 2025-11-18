# EKS Stack - AWS EKS Cluster with Pulumi Micro-stacks

[![AWS](https://img.shields.io/badge/AWS-EKS-FF9900?logo=amazon-aws)](https://aws.amazon.com/eks/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.28-326CE5?logo=kubernetes)](https://kubernetes.io/)

Micro-stack de Pulumi para aprovisionar y gestionar clústeres Amazon EKS con arquitectura modular. Este proyecto implementa Infrastructure as Code (IaC) utilizando Pulumi y Go, organizando la infraestructura en componentes independientes y reutilizables.

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

El proyecto está organizado en micro-stacks independientes que se despliegan de forma secuencial:

```
eks-stacks/
├── infra-aws/       # Infraestructura AWS base (VPC, IAM, Security)
├── infra-kube/      # Clúster EKS y node groups
├── monitoring/      # Observabilidad (Prometheus, Grafana)
├── networking/      # Ingress, CNI, certificados
└── storage/         # Bases de datos y almacenamiento
```

### Dependencias entre Stacks

```
infra-aws (VPC, IAM, Security Groups)
    ↓
infra-kube (EKS Cluster + Node Groups)
    ↓
    ├── monitoring (Prometheus, Grafana, CloudWatch)
    ├── networking (Ingress NGINX, Cilium, Cert-manager)
    └── storage (Redis, MongoDB, RDS)
```

### Comunicación entre Stacks

Los micro-stacks se comunican mediante `StackReference` de Pulumi, compartiendo outputs:

```go
// Ejemplo: monitoring/ consume outputs de infra-kube/
infraKube := pulumi.NewStackReference(ctx, 
    "organization/eks-stack-infra-kube/dev", nil)

kubeconfig := infraKube.GetStringOutput(pulumi.String("kubeconfig"))
clusterEndpoint := infraKube.GetStringOutput(pulumi.String("endpoint"))
```

## 📦 Prerrequisitos

### Herramientas Requeridas

- **AWS CLI** v2.x o superior
- **Pulumi** v3.x o superior
- **Go** 1.22 o superior
- **kubectl** v1.28 o superior
- **Helm** v3.x (opcional, para charts personalizados)

### Instalación en macOS

```bash
# AWS CLI
brew install awscli

# Pulumi
brew install pulumi

# Go
brew install go@1.22

# kubectl
brew install kubectl

# Herramientas auxiliares (recomendadas)
brew install kubectx      # Cambiar contextos de kubectl
brew install k9s          # UI terminal para Kubernetes
brew install stern        # Logs agregados de pods
```

### Instalación en Linux (Ubuntu/Debian)

```bash
# AWS CLI
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
unzip awscliv2.zip
sudo ./aws/install

# Pulumi
curl -fsSL https://get.pulumi.com | sh

# Go
sudo apt update
sudo apt install golang-1.22

# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
```

### Configuración AWS

#### 1. Configurar Perfil AWS

Crear/editar `~/.aws/config`:

```ini
[profile eks-admin]
region = us-east-1
output = json
```

#### 2. Agregar Credenciales

Editar `~/.aws/credentials`:

```ini
[eks-admin]
aws_access_key_id = YOUR_ACCESS_KEY_ID
aws_secret_access_key = YOUR_SECRET_ACCESS_KEY
```

#### 3. Verificar Configuración

```bash
aws sts get-caller-identity --profile eks-admin
```

**Output esperado:**
```json
{
    "UserId": "AIDACKCEVSQ6C2EXAMPLE",
    "Account": "123456789012",
    "Arn": "arn:aws:iam::123456789012:user/username"
}
```

## ⚙️ Configuración del Proyecto

### 1. Inicializar Backend

#### Opción A: Pulumi Cloud (Recomendado para producción)

```bash
# Login a Pulumi Cloud
pulumi login

# Verificar
pulumi whoami
```

#### Opción B: Backend Local (Para desarrollo/testing)

```bash
# Configurar backend local
export PULUMI_BACKEND_URL="file://${HOME}/.pulumi/local-eks"
pulumi login

# O usar el script helper
source ../pulumi-local.env
```

### 2. Crear Nuevo Stack

```bash
cd eks-stacks

# Crear stack desde template
./pulumi-cli.sh bootstrap
# Esto generará archivos Pulumi.<stack>.yaml para cada micro-stack
```

### 3. Configurar Stack

Editar `Pulumi.<stack-name>.yaml`:

```yaml
config:
  aws:region: us-east-1
  aws:profile: eks-admin
  
  eks-stack:
    # Configuración del cluster
    clusterName: my-eks-cluster
    kubernetesVersion: "1.28"
    
    # Configuración de red
    vpcCidr: "10.0.0.0/16"
    availabilityZones:
      - us-east-1a
      - us-east-1b
      - us-east-1c
    
    # Node groups
    nodeGroups:
      - name: general-purpose
        instanceType: t3.medium
        minSize: 2
        maxSize: 10
        desiredSize: 3
        
      - name: compute-optimized
        instanceType: c5.large
        minSize: 0
        maxSize: 5
        desiredSize: 0
```

## 🚀 Despliegue

### Preview (Sin crear recursos)

Siempre ejecutar preview antes de aplicar cambios:

```bash
# Preview con backend local (recomendado para testing)
./pulumi-cli.sh preview -s <stack-name> --local

# Preview con cloud backend (requiere credenciales AWS)
./pulumi-cli.sh preview -s <stack-name>
```

**Ejemplo de output:**
```
Previewing update (dev)

View Live: https://app.pulumi.com/org/eks-stack/dev/previews/...

     Type                        Name                    Plan       
 +   pulumi:pulumi:Stack         eks-stack-dev           create     
 +   ├─ aws:ec2:Vpc              main-vpc                create     
 +   ├─ aws:iam:Role             eks-cluster-role        create     
 +   └─ aws:eks:Cluster          eks-cluster             create     

Resources:
    + 47 to create

Duration: 12s
```

### Despliegue Completo

#### Inicializar Todos los Micro-stacks

```bash
# Este comando despliega todos los micro-stacks en orden
./pulumi-cli.sh init -s <stack-name>
```

**Orden de despliegue:**
1. **Stack principal** (orquestador) - ~2 min
2. **infra-aws** (VPC, subnets, IAM) - ~3 min
3. **infra-kube** (EKS cluster) - ~10 min
4. **monitoring** (Prometheus, Grafana) - ~3 min
5. **networking** (Ingress controller) - ~2 min
6. **storage** (Redis, MongoDB) - ~4 min

**Tiempo total estimado: ~15-20 minutos**

#### Despliegue Selectivo

Para desplegar solo un micro-stack específico:

```bash
# Desplegar solo monitoring
./pulumi-cli.sh up -s <stack-name> --sub-stack-dir monitoring

# Desplegar solo storage
./pulumi-cli.sh up -s <stack-name> --sub-stack-dir storage
```

### Actualizar Stack Existente

```bash
# Actualizar todos los micro-stacks
./pulumi-cli.sh up -s <stack-name>

# Con refresh previo (recomendado)
./pulumi-cli.sh up -s <stack-name> -r

# Con preview interactivo
./pulumi-cli.sh up -s <stack-name> -p
```

## 🔐 Acceso al Clúster

### Configurar kubectl

El script de despliegue configura kubectl automáticamente, pero también puedes hacerlo manualmente:

```bash
# Obtener kubeconfig
aws eks --region <region> update-kubeconfig \
  --name <cluster-name> \
  --profile eks-admin

# Verificar acceso
kubectl get nodes
```

**Output esperado:**
```
NAME                         STATUS   ROLES    AGE   VERSION
ip-10-0-1-123.ec2.internal   Ready    <none>   5m    v1.28.0-eks-a1b2c3d
ip-10-0-2-456.ec2.internal   Ready    <none>   5m    v1.28.0-eks-a1b2c3d
ip-10-0-3-789.ec2.internal   Ready    <none>   5m    v1.28.0-eks-a1b2c3d
```

### Contextos de kubectl

```bash
# Listar contextos disponibles
kubectx

# Cambiar a tu cluster
kubectx arn:aws:eks:us-east-1:123456789012:cluster/my-eks-cluster

# Alias más corto
kubectx eks-dev=arn:aws:eks:us-east-1:123456789012:cluster/my-eks-cluster
kubectx eks-dev

# Cambiar namespace por defecto
kubens monitoring
```

### Verificar Despliegue

```bash
# Ver todos los pods
kubectl get pods --all-namespaces

# Ver servicios importantes
kubectl get svc -n monitoring
kubectl get svc -n ingress-nginx

# Ver estado de nodes
kubectl top nodes
```

## 🛠️ Operaciones Comunes

### Ver Estado del Stack

```bash
# Ver recursos desplegados
pulumi stack -s <stack-name>

# Ver outputs del stack
pulumi stack output -s <stack-name>

# Ver output específico
pulumi stack output -s <stack-name> clusterEndpoint

# Ver historial de deployments
pulumi stack history -s <stack-name>
```

### Exportar Kubeconfig

```bash
# Exportar kubeconfig a archivo
pulumi stack output -s <stack-name> kubeconfig > kubeconfig.yaml

# Usar kubeconfig temporal
export KUBECONFIG=./kubeconfig.yaml
kubectl get nodes
```

### Refresh (Sincronizar estado)

```bash
# Refresh del stack principal
./pulumi-cli.sh refresh -s <stack-name>

# Refresh de sub-stack específico
pulumi refresh -s <stack-name> -C infra-kube
```

### Logs y Debugging

```bash
# Ver logs de un deployment
kubectl logs -n monitoring deployment/prometheus-server

# Logs en tiempo real
kubectl logs -n monitoring deployment/prometheus-server -f

# Logs de múltiples pods (stern)
stern -n monitoring prometheus

# Describir recurso
kubectl describe pod -n monitoring prometheus-server-xxx
```

### Escalar Recursos

```bash
# Escalar deployment
kubectl scale deployment -n default my-app --replicas=5

# Autoscaling
kubectl autoscale deployment -n default my-app \
  --min=2 --max=10 --cpu-percent=80
```

### Destruir Recursos

```bash
# CUIDADO: Esto eliminará todos los recursos
./pulumi-cli.sh destroy -s <stack-name>

# El script pedirá confirmación escribiendo el nombre del stack
> Destroy operation cannot be undone!
> Please confirm that this is what you'd like to do by typing 'dev':
dev

# Destruir solo un micro-stack
pulumi destroy -s <stack-name> -C storage
```

## 📂 Estructura de Micro-stacks

### infra-aws/

**Propósito**: Infraestructura AWS base y networking

**Recursos principales**:
- VPC con subnets públicas y privadas (3 AZs)
- Internet Gateway
- NAT Gateways (uno por AZ para alta disponibilidad)
- Route tables
- Security groups para EKS
- IAM roles y policies para EKS cluster y nodes

**Configuración**:
```yaml
config:
  infra-aws:
    vpcCidr: "10.0.0.0/16"
    availabilityZones: 3
    enableNatGateway: true
    singleNatGateway: false  # true para ahorro de costos
```

**Outputs**:
- `vpcId`: ID de la VPC
- `publicSubnetIds`: IDs de subnets públicas
- `privateSubnetIds`: IDs de subnets privadas
- `clusterSecurityGroupId`: Security group para EKS

**Tiempo de despliegue**: ~3 minutos

---

### infra-kube/

**Propósito**: Clúster EKS y node groups

**Recursos principales**:
- EKS cluster (control plane)
- Managed node groups con autoscaling
- EKS add-ons:
  - kube-proxy
  - coredns
  - vpc-cni
  - aws-ebs-csi-driver
- OIDC provider para IAM roles
- Cluster autoscaler

**Configuración**:
```yaml
config:
  infra-kube:
    kubernetesVersion: "1.28"
    nodeGroups:
      - name: general-purpose
        instanceType: t3.medium
        minSize: 2
        maxSize: 10
        desiredSize: 3
        diskSize: 50
        
      - name: spot-instances
        instanceType: t3.large
        minSize: 0
        maxSize: 20
        capacityType: SPOT  # Ahorro de costos
```

**Outputs**:
- `clusterEndpoint`: Endpoint del cluster
- `clusterName`: Nombre del cluster
- `kubeconfig`: Configuración para kubectl
- `clusterSecurityGroup`: Security group del cluster
- `oidcProviderArn`: ARN del OIDC provider

**Tiempo de despliegue**: ~10-12 minutos

**Dependencias**: Requiere outputs de `infra-aws/`

---

### monitoring/

**Propósito**: Observabilidad y monitoreo del cluster

**Recursos principales**:
- **Prometheus**: Métricas del cluster y aplicaciones
- **Grafana**: Visualización de métricas
- **CloudWatch Container Insights**: Integración con AWS
- **Alertmanager**: Gestión de alertas
- **Node Exporter**: Métricas de nodes
- **Kube State Metrics**: Métricas de objetos K8s

**Configuración**:
```yaml
config:
  monitoring:
    prometheusEnabled: true
    prometheusRetention: "30d"
    prometheusStorageSize: "50Gi"
    
    grafanaEnabled: true
    grafanaDomain: "grafana.example.com"
    grafanaAdminPassword: "change-me"  # Usar secrets
    
    cloudwatchEnabled: true
    
    alertmanagerUrl: "https://slack-webhook-url"
```

**Acceso a UIs**:

```bash
# Grafana (port-forward)
kubectl port-forward -n monitoring svc/grafana 3000:80
# Abrir: http://localhost:3000
# Usuario: admin / Password: <configurado>

# Prometheus
kubectl port-forward -n monitoring svc/prometheus-server 9090:80
# Abrir: http://localhost:9090

# Alertmanager
kubectl port-forward -n monitoring svc/alertmanager 9093:80
```

**Dashboards pre-configurados**:
- Cluster Overview
- Node Metrics
- Pod Resources
- Namespace Resources
- Persistent Volumes

**Tiempo de despliegue**: ~3-4 minutos

**Dependencias**: Requiere `kubeconfig` de `infra-kube/`

---

### networking/

**Propósito**: Ingress y gestión de red

**Recursos principales**:
- **NGINX Ingress Controller**: Enrutamiento HTTP/HTTPS
- **Cert-manager**: Certificados SSL/TLS automáticos (Let's Encrypt)
- **External DNS**: Gestión automática de registros DNS
- **Network Policies**: Seguridad de red a nivel de pods
- **AWS Load Balancer Controller**: Integración con ALB/NLB

**Configuración**:
```yaml
config:
  networking:
    ingressClass: nginx
    enableTLS: true
    certManagerEmail: "admin@example.com"
    
    externalDnsEnabled: true
    domainFilter: "example.com"
    
    loadBalancerType: "nlb"  # o "alb"
```

**Ejemplo de Ingress**:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
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

**Tiempo de despliegue**: ~2-3 minutos

**Dependencias**: Requiere `kubeconfig` de `infra-kube/`

---

### storage/

**Propósito**: Almacenamiento y bases de datos

**Recursos principales**:
- **Redis Cluster**: Cache distribuido
- **MongoDB**: Base de datos NoSQL
- **Amazon RDS** (opcional): PostgreSQL/MySQL gestionado
- **EBS CSI Driver**: Persistent volumes
- **Backup Cronjobs**: Respaldos automáticos

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
    
    rdsEnabled: false
    rdsInstanceClass: "db.t3.medium"
    rdsAllocatedStorage: 100
```

**Acceso a bases de datos**:

```bash
# Redis (port-forward)
kubectl port-forward -n storage svc/redis-master 6379:6379
redis-cli -h localhost

# MongoDB
kubectl port-forward -n storage svc/mongodb 27017:27017
mongo mongodb://localhost:27017
```

**Persistent Volumes**:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: app-data
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: gp3  # AWS EBS gp3
  resources:
    requests:
      storage: 10Gi
```

**Tiempo de despliegue**: ~4-5 minutos

**Dependencias**: Requiere `kubeconfig` de `infra-kube/`

---

## 🔧 Troubleshooting

### Problemas Comunes

#### 1. Error de Autenticación AWS

**Síntoma**:
```
error: Unable to connect to the server: getting credentials: exec: exit status 1
```

**Solución**:
```bash
# Verificar credenciales
aws sts get-caller-identity --profile eks-admin

# Re-configurar si es necesario
aws configure --profile eks-admin

# Actualizar kubeconfig
aws eks update-kubeconfig --name <cluster-name> --region <region> --profile eks-admin
```

#### 2. Conflicto de Estado Pulumi

**Síntoma**:
```
error: the current deployment has X resource(s) with pending operations
```

**Solución**:
```bash
# Refresh para sincronizar
pulumi refresh -s <stack-name>

# Si persiste, cancelar operaciones pendientes
pulumi cancel -s <stack-name>

# Último recurso: exportar/importar estado
pulumi stack export -s <stack-name> > state-backup.json
# Editar manualmente si es necesario
pulumi stack import -s <stack-name> < state-backup.json
```

#### 3. kubectl no puede conectar al cluster

**Síntoma**:
```
Unable to connect to the server: dial tcp: lookup XXX: no such host
```

**Solución**:
```bash
# Re-generar kubeconfig
aws eks update-kubeconfig --name <cluster-name> --region <region>

# Verificar contexto
kubectl config current-context

# Verificar que el cluster está running
aws eks describe-cluster --name <cluster-name> --region <region>
```

#### 4. Nodes no se registran en el cluster

**Síntoma**:
```
kubectl get nodes
# No nodes disponibles después de 10+ minutos
```

**Solución**:
```bash
# Ver logs de node group
aws eks describe-nodegroup \
  --cluster-name <cluster-name> \
  --nodegroup-name <nodegroup-name> \
  --region <region>

# Verificar IAM roles
# El role de los nodes debe tener estas policies:
# - AmazonEKSWorkerNodePolicy
# - AmazonEKS_CNI_Policy
# - AmazonEC2ContainerRegistryReadOnly

# Ver EC2 instances
aws ec2 describe-instances \
  --filters "Name=tag:eks:cluster-name,Values=<cluster-name>" \
  --region <region>
```

#### 5. Pods en estado Pending

**Síntoma**:
```
kubectl get pods -n monitoring
NAME                          READY   STATUS    RESTARTS   AGE
prometheus-server-xxx         0/1     Pending   0          5m
```

**Solución**:
```bash
# Verificar por qué está pending
kubectl describe pod -n monitoring prometheus-server-xxx

# Razones comunes:
# - Insufficient CPU/Memory: Escalar node group
# - Pending PVC: Verificar storage class y EBS CSI driver
# - Node selector no match: Ajustar node labels

# Escalar node group si es necesario
aws eks update-nodegroup-config \
  --cluster-name <cluster-name> \
  --nodegroup-name <nodegroup-name> \
  --scaling-config minSize=3,maxSize=10,desiredSize=5
```

#### 6. Recursos Atorados en Deletion

**Síntoma**:
```
pulumi destroy lleva >30 minutos y no termina
```

**Solución**:
```bash
# Ver qué recursos están bloqueados
pulumi stack -s <stack-name> --show-urns

# Finalizers en Kubernetes pueden bloquear
kubectl get all --all-namespaces -o json | \
  jq '.items[] | select(.metadata.deletionTimestamp != null)'

# Eliminar finalizers manualmente
kubectl patch <resource> <name> -n <namespace> \
  --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'

# Eliminar recurso del estado Pulumi (último recurso)
pulumi state delete <urn> -s <stack-name>
```

### Logs y Debugging

```bash
# Ver logs de pulumi deployment
pulumi logs -s <stack-name>

# Ver logs de pod con errores
kubectl logs -n <namespace> <pod-name> --previous

# Ver eventos del cluster
kubectl get events --all-namespaces --sort-by='.lastTimestamp'

# Shell interactivo en pod
kubectl exec -it -n <namespace> <pod-name> -- /bin/bash

# Debug de network
kubectl run tmp-shell --rm -i --tty --image nicolaka/netshoot -- /bin/bash
```

## ✅ Mejores Prácticas

### 1. Usar Preview Antes de Aplicar

```bash
# SIEMPRE revisar cambios antes de aplicar
./pulumi-cli.sh preview -s <stack-name>

# Especialmente importante para producción
./pulumi-cli.sh preview -s prod > preview-output.txt
# Revisar y compartir con el equipo antes de proceder
```

### 2. Tags y Naming Conventions

Todos los recursos incluyen tags automáticos:

```yaml
tags:
  Project: eks-stack
  Environment: dev  # o prod, staging
  ManagedBy: pulumi
  Owner: team-name
  CostCenter: engineering
```

Convención de nombres:
```
<project>-<component>-<environment>-<resource>

Ejemplos:
- eks-cluster-dev-main
- eks-nodegroup-prod-general
- eks-vpc-staging-main
```

### 3. Secrets Management

**NUNCA commitear secrets**. Usar Pulumi secrets:

```bash
# Configurar secret
pulumi config set --secret dbPassword "MySecurePassword123!" -s <stack-name>

# Ver secrets (encriptados)
pulumi config -s <stack-name>

# Usar en código
cfg := config.New(ctx, "")
password := cfg.RequireSecret("dbPassword")
```

**Alternativa: AWS Secrets Manager**

```go
secret, err := secretsmanager.NewSecret(ctx, "db-password", 
    &secretsmanager.SecretArgs{
        Description: pulumi.String("Database password"),
    })

secretVersion, err := secretsmanager.NewSecretVersion(ctx, "db-password-v1",
    &secretsmanager.SecretVersionArgs{
        SecretId:     secret.ID(),
        SecretString: pulumi.String("MySecurePassword123!"),
    })
```

### 4. Cost Optimization

```yaml
# Usar Spot instances para workloads tolerantes a fallas
nodeGroups:
  - name: spot-workers
    capacityType: SPOT
    instanceTypes:
      - t3.medium
      - t3.large
    minSize: 0
    maxSize: 20

# Single NAT Gateway para dev/staging (no para prod)
infra-aws:
  singleNatGateway: true  # Ahorra ~$90/mes

# Cluster Autoscaler para escalar a cero cuando no se usa
infra-kube:
  enableClusterAutoscaler: true
  scaleDownDelay: "10m"
```

### 5. Monitoreo y Alertas

```yaml
# Configurar alertas importantes
monitoring:
  alerts:
    - name: HighMemoryUsage
      condition: node_memory_usage > 85%
      severity: warning
      
    - name: PodCrashLooping
      condition: pod_restarts > 5
      severity: critical
      
    - name: APIServerDown
      condition: up{job="apiserver"} == 0
      severity: critical
```

### 6. Backups

```bash
# Backup de recursos importantes
./scripts/backup-resources.sh

# Backup de Pulumi state
pulumi stack export -s <stack-name> > backup-$(date +%Y%m%d).json

# Backup de etcd (si self-managed, no aplica para EKS)
# EKS gestiona backups del control plane automáticamente
```

### 7. CI/CD Integration

Ver ejemplos en `.github/workflows/` o `.gitlab-ci.yml`:

```yaml
# GitHub Actions example
name: Deploy EKS
on:
  push:
    branches: [main]
    
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: pulumi/actions@v3
        with:
          command: up
          stack-name: dev
        env:
          PULUMI_ACCESS_TOKEN: ${{ secrets.PULUMI_ACCESS_TOKEN }}
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
```

### 8. Security Hardening

```yaml
# Network policies
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all-ingress
  namespace: default
spec:
  podSelector: {}
  policyTypes:
  - Ingress

# Pod Security Standards
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

## 📚 Documentación Adicional

- [Pulumi AWS Provider](https://www.pulumi.com/registry/packages/aws/)
- [Pulumi Kubernetes Provider](https://www.pulumi.com/registry/packages/kubernetes/)
- [Amazon EKS Best Practices](https://aws.github.io/aws-eks-best-practices/)
- [Kubernetes Documentation](https://kubernetes.io/docs/)

## 🔗 Links Útiles

- [AWS EKS Workshop](https://www.eksworkshop.com/)
- [EKS Blueprints](https://aws-quickstart.github.io/cdk-eks-blueprints/)
- [Pulumi Examples](https://github.com/pulumi/examples)

---

**Parte del TFM**: "Implementación de una Estrategia Multicloud para el Despliegue de Aplicaciones Usando Pulumi Micro-stacks"