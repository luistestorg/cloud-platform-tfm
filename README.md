# Cloud Platform TFM - Multicloud Deployment with Pulumi Micro-stacks

[![Pulumi](https://img.shields.io/badge/Pulumi-3.x-8A3391?logo=pulumi)](https://www.pulumi.com/)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](https://golang.org/)
[![AWS](https://img.shields.io/badge/AWS-EKS-FF9900?logo=amazon-aws)](https://aws.amazon.com/eks/)
[![GCP](https://img.shields.io/badge/GCP-GKE-4285F4?logo=google-cloud)](https://cloud.google.com/kubernetes-engine)

Este repositorio implementa una **estrategia de despliegue multicloud** utilizando Pulumi y arquitectura de micro-stacks para la gestión de infraestructura como código (IaC) en AWS (EKS) y GCP (GKE).

## 📚 Descripción

Trabajo de Fin de Máster (TFM) que demuestra la implementación práctica de una arquitectura de micro-stacks para despliegue multicloud de aplicaciones Kubernetes. El proyecto enfatiza:

- **Modularidad**: Separación de componentes en micro-stacks independientes
- **Reutilización**: Component Resources compartidos entre clouds
- **Escalabilidad**: Patrón extensible a otros proveedores cloud
- **Medición**: KPIs cuantificables para evaluar la arquitectura

## 🏗️ Arquitectura

### Principios de Diseño

La arquitectura se basa en **micro-stacks independientes por proveedor cloud** con código compartido, siguiendo las mejores prácticas de Pulumi:

1. **Independencia de Despliegue**: Cada cloud evoluciona a su propio ritmo
2. **Aislamiento de Estado**: Reduce blast radius de errores
3. **Complejidad Gestionada**: APIs diferentes requieren separación
4. **Código DRY**: ~35% de reutilización mediante Component Resources

### Estructura del Proyecto

```
cloud-platform-tfm/
│
├── README.md                     # Este archivo
├── shared/                       # Component Resources compartidos
│   ├── components/              # Abstracciones reutilizables
│   ├── types/                   # Tipos Go compartidos
│   └── utils/                   # Funciones auxiliares
│
├── eks-stacks/                  # AWS Infrastructure
│   ├── infra-aws/              # VPC, IAM, Security
│   ├── infra-kube/             # EKS Cluster
│   ├── monitoring/             # Prometheus, Grafana
│   ├── networking/             # Ingress, Load Balancers
│   └── storage/                # Redis, MongoDB, RDS
│
└── gcp-stacks/                 # GCP Infrastructure
    ├── infra-gcp/              # VPC Network, Service Accounts
    ├── infra-kube/             # GKE Cluster
    ├── monitoring/             # Observability stack
    ├── networking/             # Load Balancing, Ingress
    ├── storage/                # Databases, Cache
    └── sql/                    # Cloud SQL
```

### Flujo de Dependencias

```
┌─────────────────┐     ┌─────────────────┐
│   infra-aws     │     │   infra-gcp     │
│  (VPC, IAM)     │     │  (VPC, IAM)     │
└────────┬────────┘     └────────┬────────┘
         │                       │
         ▼                       ▼
┌─────────────────┐     ┌─────────────────┐
│  infra-kube     │     │  infra-kube     │
│  (EKS Cluster)  │     │  (GKE Cluster)  │
└────────┬────────┘     └────────┬────────┘
         │                       │
         ├───────┬───────┐       ├───────┬───────┐
         ▼       ▼       ▼       ▼       ▼       ▼
    monitoring networking storage  monitoring networking storage
```

## 🚀 Getting Started

### Prerequisitos

**Herramientas requeridas:**

- [Pulumi](https://www.pulumi.com/docs/install/) v3.x o superior
- [Go](https://golang.org/doc/install) 1.22 o superior
- [AWS CLI](https://aws.amazon.com/cli/) v2.x
- [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) v400.0.0+
- [kubectl](https://kubernetes.io/docs/tasks/tools/) v1.28+

**Cuentas cloud:**
- AWS Account con permisos de administrador
- GCP Project con APIs habilitadas

### Instalación Rápida (macOS)

```bash
# Instalar herramientas
brew install pulumi go@1.22 awscli google-cloud-sdk kubectl

# Verificar instalación
pulumi version
go version
aws --version
gcloud --version
kubectl version --client
```

### Configuración Inicial

#### 1. Clonar Repositorio

```bash
git clone https://github.com/your-org/cloud-platform-tfm.git
cd cloud-platform-tfm
```

#### 2. Configurar Credenciales AWS

```bash
# Configurar perfil AWS
aws configure --profile eks-admin

# Verificar acceso
aws sts get-caller-identity --profile eks-admin
```

#### 3. Configurar Credenciales GCP

```bash
# Login a GCP
gcloud auth login
gcloud auth application-default login

# Configurar proyecto
gcloud config set project YOUR_PROJECT_ID

# Habilitar APIs necesarias
gcloud services enable compute.googleapis.com
gcloud services enable container.googleapis.com
```

#### 4. Pulumi Backend

**Opción A: Pulumi Cloud (Recomendado para producción)**

```bash
pulumi login
```

**Opción B: Backend Local (Para desarrollo/testing)**

```bash
# Configurar backend local
export PULUMI_BACKEND_URL="file://${HOME}/.pulumi/local"
pulumi login

# O usar script helper
source pulumi-local.env
```

## 📖 Documentación

### Por Cloud Provider

- **[eks-stacks/README.md](eks-stacks/README.md)**: Documentación completa para AWS EKS
- **[gcp-stacks/README.md](gcp-stacks/README.md)**: Documentación completa para GCP GKE

### Componentes Compartidos

- **[shared/README.md](shared/README.md)**: Component Resources reutilizables

### Guías Adicionales

- **Despliegue**: Ver secciones de deployment en cada README
- **Testing Local**: Usar `pulumi preview --local` para validar sin crear recursos
- **Troubleshooting**: Consultar secciones específicas en cada README

## 🎯 Uso

### Preview (Sin Crear Recursos)

```bash
# Preview en EKS (AWS)
cd eks-stacks
./pulumi-cli.sh preview -s dev --local

# Preview en GKE (GCP)
cd gcp-stacks
./pulumi-cli.sh preview -s dev --local
```

### Despliegue Completo

#### AWS EKS

```bash
cd eks-stacks

# Inicializar todos los micro-stacks
./pulumi-cli.sh init -s dev

# O actualizar stack existente
./pulumi-cli.sh up -s dev
```

#### GCP GKE

```bash
cd gcp-stacks

# Configurar variables de entorno requeridas
export OAUTH2_CLIENT_SECRET="..."
export SLACK_WEBHOOK_URL="..."

# Inicializar todos los micro-stacks
./pulumi-cli.sh init -s dev
```

### Acceso a Clústeres

#### AWS EKS

```bash
# Configurar kubectl
aws eks --region us-east-1 update-kubeconfig \
  --name <cluster-name> \
  --profile eks-admin

# Verificar acceso
kubectl get nodes
```

#### GCP GKE

```bash
# Configurar kubectl
gcloud container clusters get-credentials <cluster-name> \
  --region us-central1 \
  --project <project-id>

# Verificar acceso
kubectl cluster-info
```

## 📊 Métricas y KPIs

### Categorías de Evaluación

El proyecto mide KPIs en 5 categorías:

1. **Rendimiento Operacional**: Tiempo de despliegue, preview, MTTR
2. **Arquitectura y Modularidad**: Número de micro-stacks, líneas/stack, complejidad
3. **Calidad de Código**: Código compartido %, duplicación, test coverage
4. **Capacidades Multicloud**: Portabilidad, consistencia, tiempo de migración
5. **Escalabilidad**: Tiempo para agregar recursos, extensibilidad

### Ejemplo de Comparación

```
┌────────────────────────────┬────────────┬────────────┬──────────────┐
│ KPI                        │ AWS EKS    │ GCP GKE    │ Diferencia   │
├────────────────────────────┼────────────┼────────────┼──────────────┤
│ Tiempo deployment (min)    │ 15         │ 12         │ -20%         │
│ Líneas código por stack    │ ~250       │ ~230       │ -8%          │
│ Código compartido (%)      │ 35%        │ 35%        │ 0%           │
│ Complejidad ciclomática    │ 12         │ 11         │ -8.3%        │
│ Test coverage (%)          │ 75%        │ 78%        │ +4%          │
└────────────────────────────┴────────────┴────────────┴──────────────┘
```

## 🧪 Testing

### Unit Tests

```bash
# Tests de componentes compartidos
cd shared
go test ./... -v

# Tests por micro-stack
cd eks-stacks/monitoring
go test ./... -v
```

### Integration Tests

```bash
# Preview con backend local (no crea recursos reales)
cd eks-stacks
./pulumi-cli.sh preview -s test --local
```

## 🔧 Desarrollo

### Agregar Nuevo Micro-stack

1. Crear estructura de directorios:
```bash
mkdir -p eks-stacks/new-component
cd eks-stacks/new-component
```

2. Copiar template:
```bash
cp ../monitoring/Pulumi.yaml .
cp ../monitoring/Pulumi.stack-yaml.gotmpl .
```

3. Crear `main.go` siguiendo el patrón de otros stacks

4. Actualizar `pulumi-cli.sh` para incluir el nuevo stack

### Usar Component Resources Compartidos

```go
import "your-module/shared/components"

func main() {
    pulumi.Run(func(ctx *pulumi.Context) error {
        // Usar component compartido
        monitoring, err := components.NewMonitoringStack(ctx, "my-monitoring",
            &components.MonitoringStackArgs{
                CloudType: "aws", // o "gcp"
                // ... otros args
            })
        
        return err
    })
}
```

## 🤝 Contribución

Este es un proyecto académico (TFM), pero las contribuciones son bienvenidas:

1. Fork el repositorio
2. Crear feature branch (`git checkout -b feature/amazing-feature`)
3. Commit cambios (`git commit -m 'Add amazing feature'`)
4. Push a branch (`git push origin feature/amazing-feature`)
5. Abrir Pull Request

## 📄 Licencia

Este proyecto es parte de un Trabajo de Fin de Máster (TFM) y se distribuye con fines académicos.

## 👥 Autor

**Luis Ccari**
- Máster en Ingeniería de Software
- Universidad: [Nombre de Universidad]
- Email: [tu-email]
- LinkedIn: [tu-linkedin]

## 🙏 Agradecimientos

- [Pulumi](https://www.pulumi.com/) por su excelente framework de IaC
- Comunidad de Pulumi por las mejores prácticas documentadas
- Referencias específicas en bibliografía del TFM

## 📚 Referencias

- [Pulumi Documentation - Organizing Projects & Stacks](https://www.pulumi.com/docs/iac/using-pulumi/organizing-projects-stacks/)
- [HUMAN Security - Micro-stacks vs Monolithic](https://www.humansecurity.com/tech-engineering-blog/pulumi-approaches-micro-stacks-vs-monolithic-stack/)
- [Pulumi Blog - IaC Best Practices](https://www.pulumi.com/blog/iac-recommended-practices-code-organization-and-stacks/)
- [Multicloud Kubernetes with Pulumi](https://www.pulumi.com/blog/multicloud-app/)

## 🔗 Enlaces Útiles

- [Documentación del Proyecto](docs/)
- [Issues](https://github.com/luis-munoz/cloud-platform-tfm/issues)
- [Wiki](https://github.com/luis-munoz/cloud-platform-tfm/wiki)
- [Pulumi Registry](https://www.pulumi.com/registry/)

---

**Nota**: Este README es parte del TFM "Implementación de una Estrategia Multicloud para el Despliegue de Aplicaciones Usando Pulumi Micro-stacks"