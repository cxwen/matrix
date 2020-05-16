# kubernetes安装

## node节点kubelet及kubeadm安装

1、配置阿里云kubernetes yum源

```bash
cat > /etc/yum.repos.d/kubernetes.repo << EOF
[kubernetes]
name=Kubernetes
baseurl=https://mirrors.aliyun.com/kubernetes/yum/repos/kubernetes-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://mirrors.aliyun.com/kubernetes/yum/doc/yum-key.gpg https://mirrors.aliyun.com/kubernetes/yum/doc/rpm-package-key.gpg
EOF

yum makecache
```

2、查看支持版本

```bash
yum list kubelet --showduplicates
```

3、安装

```bash
k8sVersion=1.15.12
yum install -y kubelet-${k8sVersion} kubeadm-${k8sVersion} kubectl-${k8sVersion}
```

```bash
systemctl enable kubelet
```
