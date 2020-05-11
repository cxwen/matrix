# matrix

将kubernetes集群的master、etcd组件运行在已有的kubernetes集群中

![](./matrix.jpg)

# 快速开始

母集群：在使用之前，需要先有一个已运行的kubernetes集群，版本：v1.15.+

``` shell
kubectl apply -f config/crd.yaml
```


