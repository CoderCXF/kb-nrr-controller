![image-20220312114610107](C:\Users\CXF\AppData\Roaming\Typora\typora-user-images\image-20220312114610107.png)

APiServer是k8s的入口点，如论是通过kubectl 还是client-go都是最后请求的ApiServer。

CRD是用户自己定义的资源，对自己的资源的管理就是Controller,自己写的Conrtoller对自己的资源管理。内置资源的管理比如deployment都是有的.

client-go就是一个独立的API，是不归k8s管理的。当然也可以使用k8s提供的一些其他的。