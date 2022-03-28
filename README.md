# kb-nrr-controller

* 使用kubebuilder实现的Operator；
* 使用metric-server进行CPU和Memory资源统计；
* 自定义CRD资源的yaml文件在namespace-resource-report文件夹下。

问题：

1. 在更新和删除pod的时候调谐函数不能捕捉到事件；

2. 不使用kubebuilder工具，基于sample-controller实现任务要求的Operator可以捕获到事件。但是不知道如何写Dockerfile部署在集群中。

