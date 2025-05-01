## 0 前言

几周前和大家一起走读了 grpc-go 客户端的源码链路，本篇则是想着重探讨一下其中涉及到的“服务发现”以及“负载均衡”的相关内容. 本文会贴近于生产环境，使用到分布式存储组件 etcd 作为 grpc 服务的注册与发现模块，并引用 [roundRobin](https://zhida.zhihu.com/search?content_id=226797614&content_type=Article&match_order=1&q=roundRobin&zhida_source=entity) 轮询算法作为负载均衡策略.

## 1 背景

## 1.1 grpc 源码

本系列探讨的主题是由 google 研发的开源 rpc 框架 grpc-go.

对应的开源地址为：[https://github.com/grpc/grpc-go/](https://link.zhihu.com/?target=https%3A//github.com/grpc/grpc-go/) . 走读的源码版本为 Release 1.54.0.

![img](https://picx.zhimg.com/v2-456a94ec64b4f42442fefd6ba36f5885_1440w.jpg)



## 1.2 grpc 负载均衡

C-S 架构中负载均衡策略可以分为两大类——基于服务端实现负载均衡的模式以及基于客户端实现负载均衡的模式.

grpc-go 中采用的是基于服务端实现负载均衡的模式. 在这种模式下，客户端会首先取得服务端的节点（endpoint）列表，然后基于一定的负载均衡策略选择到特定的 endpoint，然后直连发起请求.

![img](https://pic1.zhimg.com/v2-07b562175c6b80172bad09f0b3c7dbce_1440w.jpg)



## 1.3 etcd 服务注册与发现

etcd是一个分布式 KV 存储组件，协议层通过 [raft 算法](https://zhida.zhihu.com/search?content_id=226797614&content_type=Article&match_order=1&q=raft+算法&zhida_source=entity)保证了服务的强一致性和高可用性，同时，etcd 还提供了针对于存储数据的 watch 监听回调功能，基于这一特性，etcd 很适合用于作为配置管理中心或者服务注册/发现模块.

etcd 的开源地址为 [https://github.com/etcd-io/etcd](https://link.zhihu.com/?target=https%3A//github.com/etcd-io/etcd)

本文走读的 etcd 源码版本为 v3.5.8.

![img](https://pic3.zhimg.com/v2-3125b5f199b11516e6d9702a8412072c_1440w.jpg)



在使用 etcd 作为服务注册/发现模块时，同一个服务组在 etcd 中会以相同的服务名作为共同的标识键前缀，与各服务节点的信息建立好映射关系，以实现所谓的“服务注册”功能.

在客户端使用“服务发现”功能时，则会在 etcd 中通过服务名取得对应的服务节点列表缓存在本地，然后在客户端本地基于负载均衡策略选择 endpoint 进行连接请求. 在这个过程中，客户端还会利用到 etcd 的 [watch 功能](https://zhida.zhihu.com/search?content_id=226797614&content_type=Article&match_order=1&q=watch+功能&zhida_source=entity)，在服务端节点发生变化时，及时感知到变更事件，然后对本地缓存的服务端节点列表进行更新.

![img](https://pic4.zhimg.com/v2-101e7c05d88ee33601c7093473555f1d_1440w.jpg)



## 1.4 [etcd-grpc](https://zhida.zhihu.com/search?content_id=226797614&content_type=Article&match_order=1&q=etcd-grpc&zhida_source=entity)

![img](https://pica.zhimg.com/v2-1b1c3c8273af249ba0d70f8df88a0834_1440w.jpg)



etcd 是用 go 语言编写的，和 grpc-go 具有很好的兼容性. 在 etcd 官方文档中就给出了在 grpc-go 服务中使用 etcd 作为服务注册/发现模块的示例，参考文档见：[https://etcd.io/docs/v3.5/dev-guide/grpc_naming/](https://link.zhihu.com/?target=https%3A//etcd.io/docs/v3.5/dev-guide/grpc_naming/) .



![img](https://pica.zhimg.com/v2-9d3f5bf891bbb971435a24f59a7ca0d2_1440w.jpg)



官方文档的使用示例在作为本文源码走读的方法入口，下面开始.

## 2 服务端

## 2.1 启动入口

首先给出，grpc-go服务端启动并通过 etcd 实现服务注册的代码示例.

```text
package main


import (
    // 标准库
    "context"
    "flag"
    "fmt"
    "net"
    "time"


    // grpc 桩代码
    "github.com/grpc_demo/proto"


    // etcd
    eclient "go.etcd.io/etcd/client/v3"
    "go.etcd.io/etcd/client/v3/naming/endpoints"


    // grpc
    "google.golang.org/grpc"
)


const (
    // grpc 服务名
    MyService = "xiaoxu/demo"
    // etcd 端口
    MyEtcdURL = "http://localhost:2379"
)


type Server struct {
    proto.UnimplementedHelloServiceServer
}


func main() {
    // 接收命令行指定的 grpc 服务端口
    var port int
    flag.IntVar(&port, "port", 8080, "port")
    flag.Parse()
    addr := fmt.Sprintf("http://localhost:%d", port)


    // 创建 tcp 端口监听器
    listener, _ := net.Listen("tcp", addr)
    
    // 创建 grpc server
    server := grpc.NewServer()
    proto.RegisterHelloServiceServer(server, &Server{})


    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    // 注册 grpc 服务节点到 etcd 中
    go registerEndPointToEtcd(ctx, addr)
   
    // 启动 grpc 服务
    if err := server.Serve(listener); err != nil {
        fmt.Println(err)
    }
}
```

## 2.2 服务注册

registerEndPointToEtcd 方法给出了将 grpc 服务端节点注册到 etcd 模块的示例：

- eclient.NewFromURL 创建 etcd 客户端 etcdClient
- endpoints.NewManager 创建 etcd 服务端节点管理模块 etcdManager
- etcdClient.Grant 申请一份租约，租约的有效时间为 ttl
- etcdManager.AddEndpoint 将当前节点注册到 etcd 中，同时会和租约进行关联
- etcdClient.KeepAliveOnce 对租约进行一轮续期，重置租约失效的 ttl

```text
func registerEndPointToEtcd(ctx context.Context, addr string) {
    // 创建 etcd 客户端
    etcdClient, _ := eclient.NewFromURL(MyEtcdURL)
    etcdManager, _ := endpoints.NewManager(etcdClient, MyService)


    // 创建一个租约，每隔 10s 需要向 etcd 汇报一次心跳，证明当前节点仍然存活
    var ttl int64 = 10
    lease, _ := etcdClient.Grant(ctx, ttl)
    
    // 添加注册节点到 etcd 中，并且携带上租约 id
    _ = etcdManager.AddEndpoint(ctx, fmt.Sprintf("%s/%s", MyService, addr), endpoints.Endpoint{Addr: addr}, eclient.WithLease(lease.ID))


    // 每隔 5 s进行一次延续租约的动作
    for {
        select {
        case <-time.After(5 * time.Second):
            // 续约操作
            resp, _ := etcdClient.KeepAliveOnce(ctx, lease.ID)
            fmt.Printf("keep alive resp: %+v", resp)
        case <-ctx.Done():
            return
        }
    }
}
```

![img](https://pica.zhimg.com/v2-96202f1118989e6efc6428fd9c262f1a_1440w.jpg)



## 2.4 注册节点

![simg](https://picx.zhimg.com/v2-b3a603f2fc9037a2677ae37b9f192375_1440w.png)



在 grpc 服务端注册 endpoint 时，调用了方法链 endpointManager.AddEndpoint -> endpointManager.Update，将服务节点 endpoint 以共同的服务名作为标识键 key 的前缀，添加到 kv 存储介质当中.

由于 endpoint 的注册信息关联到了租约，因此倘若租约过期，endpoint 的注册信息也随之失效. 所以 endpoint 在运行过程中，需要持续向 etcd 发送心跳以进行租约的续期，背后的作用正是通过这种续约机制向 etcd 服务注册模块证明 endpoint 自身的仍然处于存活状态.

```text
func (m *endpointManager) AddEndpoint(ctx context.Context, key string, endpoint Endpoint, opts ...clientv3.OpOption) error {
    return m.Update(ctx, []*UpdateWithOpts{NewAddUpdateOpts(key, endpoint, opts...)})
}
func (m *endpointManager) Update(ctx context.Context, updates []*UpdateWithOpts) (err error) {
    ops := make([]clientv3.Op, 0, len(updates))
    for _, update := range updates {
        if !strings.HasPrefix(update.Key, m.target+"/") {
            return status.Errorf(codes.InvalidArgument, "endpoints: endpoint key should be prefixed with '%s/' got: '%s'", m.target, update.Key)
        }


        switch update.Op {
        case Add:
            internalUpdate := &internal.Update{
                Op:       internal.Add,
                Addr:     update.Endpoint.Addr,
                Metadata: update.Endpoint.Metadata,
            }


            var v []byte
            if v, err = json.Marshal(internalUpdate); err != nil {
                return status.Error(codes.InvalidArgument, err.Error())
            }
            ops = append(ops, clientv3.OpPut(update.Key, string(v), update.Opts...))
        // ...
        }
    }
    _, err = m.client.KV.Txn(ctx).Then(ops...).Commit()
    return err
}
```

## 3 客户端

![img](https://pic4.zhimg.com/v2-c842a0159d6bcee06dcf5772a3443acd_1440w.jpg)



首先晒一下前文——grpc-go 客户端源码走读 展示过的客户端架构图，这些内容会为本文的展开打下铺垫.

## 3.1 启动入口

下面给出 grpc-go 客户端启动的代码示例，核心点的注释已经给出.

```text
package main

import (
    // 标准库
    "context"
    "fmt"
    "time"

    // grpc 桩文件
    "github.com/grpc_demo/proto"

    // etcd
    eclient "go.etcd.io/etcd/client/v3"
    eresolver "go.etcd.io/etcd/client/v3/naming/resolver"


    // grpc
    "google.golang.org/grpc"
    "google.golang.org/grpc/balancer/roundrobin"
    "google.golang.org/grpc/credentials/insecure"
)

const MyService = "xiaoxu/demo"

func main() {
    // 创建 etcd 客户端
    etcdClient, _ := eclient.NewFromURL("my_etcd_url")
    
    // 创建 etcd 实现的 grpc 服务注册发现模块 resolver
    etcdResolverBuilder, _ := eresolver.NewBuilder(etcdClient)
    
    // 拼接服务名称，需要固定义 etcd:/// 作为前缀
    etcdTarget := fmt.Sprintf("etcd:///%s", MyService)
    
    // 创建 grpc 连接代理
    conn, _ := grpc.Dial(
        // 服务名称
        etcdTarget,
        // 注入 etcd resolver
        grpc.WithResolvers(etcdResolverBuilder),
        // 声明使用的负载均衡策略为 roundrobin     grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"LoadBalancingPolicy": "%s"}`, roundrobin.Name)),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    defer conn.Close()

    // 创建 grpc 客户端
    client := proto.NewHelloServiceClient(conn)
    for {
        // 发起 grpc 请求
        resp, _ := client.SayHello(context.Background(), &proto.HelloReq{
            Name: "xiaoxuxiansheng",
        })
        fmt.Printf("resp: %+v", resp)
        // 每隔 1s 发起一轮请求
        <-time.After(time.Second)
    }
}
```

## 3.2 注入 etcd resolver

在 grpc 客户端启动时，首先会获取到 etcd 中提供的 grpc 服务发现构造器 resolverBuilder，然后在调用 grpc.Dial 方法创建连接代理 ClientConn 时，将其注入其中.

```text
func main() {
    // ...
    // 创建 etcd 实现的 grpc 服务注册发现模块 resolver
    etcdResolverBuilder, _ := eresolver.NewBuilder(etcdClient)
    
    // ...
    // 创建 grpc 连接代理
    conn, _ := grpc.Dial(
        // ...
        // 注入 etcd resolver
        grpc.WithResolvers(etcdResolverBuilder),
        // ...
    )
    // ...
}
func WithResolvers(rs ...resolver.Builder) DialOption {
    return newFuncDialOption(func(o *dialOptions) {
        o.resolvers = append(o.resolvers, rs...)
    })
}
```

etcd 实现的 resolverBuilder 源码如下，其中内置了一个 etcd 客户端用于获取 endpoint 注册信息. etcdResolverBuilder 的 schema 是 ”etcd“，因此后续在通过 etcd 作为服务发现模块时，使用的服务名标识键需要以 etcd 作为前缀.

```text
type builder struct {
    c *clientv3.Client
}


func (b builder) Scheme() string {
    return "etcd"
}


// NewBuilder creates a resolver builder.
func NewBuilder(client *clientv3.Client) (gresolver.Builder, error) {
    return builder{c: client}, nil
}
```

## 3.3 启动 grpc balancerWrapper

![img](https://pica.zhimg.com/v2-419b24adb546b7ab393561509534e7de_1440w.jpg)



在 grpc-go 客户端启动时，会调用方法链 DialContext -> newCCBalancerWrapper -> go ccBalancerWrapper.watcher，启动一个 balancerWrapper 的守护协程，持续监听 ClientConn 更新、balancer 更新等事件并进行处理.

```text
func DialContext(ctx context.Context, target string, opts ...DialOption) (conn *ClientConn, err error) {
    // ...
    cc.balancerWrapper = newCCBalancerWrapper(cc, balancer.BuildOptions{
        DialCreds:        credsClone,
        CredsBundle:      cc.dopts.copts.CredsBundle,
        Dialer:           cc.dopts.copts.Dialer,
        Authority:        cc.authority,
        CustomUserAgent:  cc.dopts.copts.UserAgent,
        ChannelzParentID: cc.channelzID,
        Target:           cc.parsedTarget,
    })


    // ...
    return cc, nil
}
func newCCBalancerWrapper(cc *ClientConn, bopts balancer.BuildOptions) *ccBalancerWrapper {
    ccb := &ccBalancerWrapper{
        cc:       cc,
        updateCh: buffer.NewUnbounded(),
        resultCh: buffer.NewUnbounded(),
        closed:   grpcsync.NewEvent(),
        done:     grpcsync.NewEvent(),
    }
    go ccb.watcher()
    ccb.balancer = gracefulswitch.NewBalancer(ccb, bopts)
    return ccb
}
```

![img](https://pica.zhimg.com/v2-3a59f74b043f0c9e9494a4738390eb06_1440w.jpg)



```text
func (ccb *ccBalancerWrapper) watcher() {
    for {
        select {
        case u := <-ccb.updateCh.Get():
            // ...
            switch update := u.(type) {
            case *ccStateUpdate:
                ccb.handleClientConnStateChange(update.ccs)
            case *switchToUpdate:
                ccb.handleSwitchTo(update.name)                
            // ...
            }
        case <-ccb.closed.Done():
        }
        // ...
    }
}
```

## 3.4 获取 etcd resolver builder

![img](https://pica.zhimg.com/v2-cdb95fcc76f5da5231a8a9026bb8bd92_1440w.jpg)



在 grpc-go 客户端启动时，还有一条方法链路是 DialContext -> ClientConn.parseTargetAndFindResolver -> ClientConn.getResolver，通过 target 的 schema（etcd），读取此前通过 option 注入的 etcd resolverBuilder.

```text
func DialContext(ctx context.Context, target string, opts ...DialOption) (conn *ClientConn, err error) {
    // ...
    // Determine the resolver to use.
    resolverBuilder, err := cc.parseTargetAndFindResolver()  
    // ...
    rWrapper, err := newCCResolverWrapper(cc, resolverBuilder)
    // ...
}
func (cc *ClientConn) parseTargetAndFindResolver() (resolver.Builder, error) {
    // ...
    var rb resolver.Builder
    // ...
    rb = cc.getResolver(parsedTarget.URL.Scheme)
    // ...
    return rb, nil
    // ...
}
func (cc *ClientConn) getResolver(scheme string) resolver.Builder {
    for _, rb := range cc.dopts.resolvers {
        if scheme == rb.Scheme() {
            return rb
        }
    }
    return resolver.Get(scheme)
}
```

## 3.5 创建并启动 etcd resolver

![img](https://pic4.zhimg.com/v2-ac2341b851baba61610050a8ae94504b_1440w.jpg)



在取得 etcd resolver builder 后，会在 newCCResolverWrapper 方法中，执行 builder.Build 方法进行 etcd resolver 的创建.

```text
func DialContext(ctx context.Context, target string, opts ...DialOption) (conn *ClientConn, err error) {
    // ...
    // Build the resolver.
    rWrapper, err := newCCResolverWrapper(cc, resolverBuilder)
    // ...


    return cc, nil
}
func newCCResolverWrapper(cc *ClientConn, rb resolver.Builder) (*ccResolverWrapper, error) {
    // ...
    ccr.resolver, err = rb.Build(cc.parsedTarget, ccr, rbo)
    // ...
}
```

被构建出来的 etcd resolver 定义如下：

```text
type resolver struct {
    // etcd 客户端
    c      *clientv3.Client
    target string
    // grpc 连接代理
    cc     gresolver.ClientConn
    // 持续监听的 etcd chan，能够获取到服务端 endpoint 的变更事件
    wch    endpoints.WatchChannel
    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
}
```

在 etcd resolver builder 构建 resolver 的过程中，会获取到一个来自 etcd 客户端的 channel，用于持续监听 endpoint 的变更事件，以维护更新客户端缓存的 endpoint 列表.

构建出 resolver 后，会调用 go resolver.watch 方法开启一个守护协程，持续监听 channel.

```text
func (b builder) Build(target gresolver.Target, cc gresolver.ClientConn, opts gresolver.BuildOptions) (gresolver.Resolver, error) {
    r := &resolver{
        c:      b.c,
        target: target.Endpoint,
        cc:     cc,
    }
    r.ctx, r.cancel = context.WithCancel(context.Background())


    // 创建 etcd endpoint 管理服务实例
    em, err := endpoints.NewManager(r.c, r.target)
    // 获取 endpoint 变更事件的监听 channel
    r.wch, err = em.NewWatchChannel(r.ctx)
    
    // ...
    r.wg.Add(1)
    // 开启对 endpoint 变更事件的监听
    go r.watch()
    return r, nil
}
```

![img](https://pic2.zhimg.com/v2-2efc1882ba4f3e18aa3dcd6bc0f54613_1440w.png)



在守护协程 watcher 中，每当感知到 endpoint 的变更，则会此时全量的 endpoints 作为入参，通过调用 ccResolverWrapper.UpdateState 方法对 grpc 连接代理 ClientConn 进行更新，保证 grpc-go 客户端维护到最新的 endpoint 实时数据.

```text
func (r *resolver) watch() {
    defer r.wg.Done()


    allUps := make(map[string]*endpoints.Update)
    for {
        select {
        case <-r.ctx.Done():
            return
        // 监听到 grpc 服务端 endpoint 变更事件
        case ups, ok := <-r.wch:
            // ...
            // 处理监听事件
            for _, up := range ups {
                switch up.Op {
                case endpoints.Add:
                    allUps[up.Key] = up
                case endpoints.Delete:
                    delete(allUps, up.Key)
                }
            }
  
            addrs := convertToGRPCAddress(allUps)
            // 监听到 endpoint 变更时，需要将其更新到 grpc 客户端本地维护的 subConns 列表当中
            r.cc.UpdateState(gresolver.State{Addresses: addrs})
        }
    }
}
```

## 3.6 接收 endpoint 更新事件

![img](https://pic4.zhimg.com/v2-2f3145ef336ca5211925a850eebaaf67_1440w.jpg)



在 etcd resolver 的守护协程接收到 endpoint 变更事件后，会经历 ccResolverWrapper.UpdateState -> ClientConn.updateResolverState 方法链路的调用，其中会执行两项任务：

- 倘若还没设置过负载均衡器，则会对其进行设置（本次使用到的负载均衡策略为 round-robin 轮询算法）——ClientConn.maybeApplyDefaultServiceConfig
- 对负载均衡器中缓存的 endpoint 数据进行更新——ccBalancerWrapper.updateClientConnState

```text
func (ccr *ccResolverWrapper) UpdateState(s resolver.State) error {
    // ...
    if err := ccr.cc.updateResolverState(ccr.curState, nil); err == balancer.ErrBadResolverState {
    // ...
}
func (cc *ClientConn) updateResolverState(s resolver.State, err error) error {
    // ...
    if s.ServiceConfig == nil {
        cc.maybeApplyDefaultServiceConfig(s.Addresses)
    }
    // ...
    uccsErr := bw.updateClientConnState(&balancer.ClientConnState{ResolverState: s, BalancerConfig: balCfg})
    // ...
    return ret
}
```

## 3.7 启用 roundrobin balancer

![img](https://pic3.zhimg.com/v2-3c588b7ea57d671404b2bb101982f570_1440w.jpg)



首先聊聊 grpc 客户端启用负载均衡器 round-robin 的链路.

经由 ClientConn.maybeApplyDefaultServiceConfig -> ClientConn.applyServiceConfigAndBalancer -> balancerWrapper.switchTo 的方法链路，会通过 grpc 客户端启动时注入的 defaultServiceConfig 中获取本次使用的负载均衡策略名 "round_robin"，接下来会调用 ccBalancerWrapper.switchTo 方法，将当前使用的负载均衡器切换成 round-robin 类型.

```text
func (cc *ClientConn) maybeApplyDefaultServiceConfig(addrs []resolver.Address) {
    // ...
    if cc.dopts.defaultServiceConfig != nil {
        cc.applyServiceConfigAndBalancer(cc.dopts.defaultServiceConfig, &defaultConfigSelector{cc.dopts.defaultServiceConfig}, addrs)
    } 
    // ...
}
func (cc *ClientConn) applyServiceConfigAndBalancer(sc *ServiceConfig, configSelector iresolver.ConfigSelector, addrs []resolver.Address) {
    // ... 
    cc.sc = sc
    if configSelector != nil {
        cc.safeConfigSelector.UpdateConfigSelector(configSelector)
    }




    // ...
 
    // 读取配置，设定 newBalancer 为 defaultServiceConfig 中传入的 roundrobin
    var newBalancerName string
    // ...
    if cc.sc != nil && cc.sc.LB != nil {
        newBalancerName = *cc.sc.LB
    } 
    cc.balancerWrapper.switchTo(newBalancerName)
}
```

在 ccBalancerWrapper 守护协程 watcher 接收到 switchToUpdate 类型的变更事件后，会顺沿 ccBalancerWrapper.handleSwtichTo -> Balancer.SwitchTo -> baseBuilder.Build 的方法链路，最终真正构造出 round-robin 类型的负载均衡器，此时 baseBalancer 中内置的关键字段 pickerBuilder 为 rrPickerBuilder（rr 为 round-robin 的简写）.

```text
func (ccb *ccBalancerWrapper) handleSwitchTo(name string) {
    // ...
    // 从全局 balancer map 中获取 roundrobin 对应的 balancerBuilder
    builder := balancer.Get(name)
    // ...


    if err := ccb.balancer.SwitchTo(builder); err != nil {
        // ...
        return
    }
    ccb.curBalancerName = builder.Name()
}
func (gsb *Balancer) SwitchTo(builder balancer.Builder) error {
    // ...
    bw := &balancerWrapper{
        gsb: gsb,
        lastState: balancer.State{
            ConnectivityState: connectivity.Connecting,
            Picker:            base.NewErrPicker(balancer.ErrNoSubConnAvailable),
        },
        subconns: make(map[balancer.SubConn]bool),
    }
    
    // ...
    // 创建 roundrobin balancer
    newBalancer := builder.Build(bw, gsb.bOpts)
    // ...
    bw.Balancer = newBalancer
    return nil
}
func (bb *baseBuilder) Build(cc balancer.ClientConn, opt balancer.BuildOptions) balancer.Balancer {
    bal := &baseBalancer{
        cc:            cc,
        pickerBuilder: bb.pickerBuilder,


        subConns: resolver.NewAddressMap(),
        scStates: make(map[balancer.SubConn]connectivity.State),
        csEvltr:  &balancer.ConnectivityStateEvaluator{},
        config:   bb.config,
        state:    connectivity.Connecting,
    }
    // ...
    bal.picker = NewErrPicker(balancer.ErrNoSubConnAvailable)
    return bal
}
```

## 3.8 更新 endpoint

![img](https://pica.zhimg.com/v2-9ce702dcabf893098d5b20febedc4138_1440w.jpg)



接下来需要对 grpc 客户端负载均衡器 balancer 中的 endpoint 信息进行更新.

方法链路为 ccBalancerWrapper -> handleClientConnStateChange -> Balancer.UpdateClientConnState -> baseBalancer.UpdateClientConnState

其中，会获取到实时的全量 endpoints 数据，然后调用 baseBalancer.regeneratePicker 方法进行 rrPicker 的重铸，并且将最新的数据注入其中.

```text
func (ccb *ccBalancerWrapper) updateClientConnState(ccs *balancer.ClientConnState) error {
    ccb.updateCh.Put(&ccStateUpdate{ccs: ccs})
    // ...
}
func (ccb *ccBalancerWrapper) handleClientConnStateChange(ccs *balancer.ClientConnState) {
    if ccb.curBalancerName != grpclbName {
        // Filter any grpclb addresses since we don't have the grpclb balancer.
        var addrs []resolver.Address
        for _, addr := range ccs.ResolverState.Addresses {
            if addr.Type == resolver.GRPCLB {
                continue
            }
            addrs = append(addrs, addr)
        }
        ccs.ResolverState.Addresses = addrs
    }
    ccb.resultCh.Put(ccb.balancer.UpdateClientConnState(*ccs))
}
func (gsb *Balancer) UpdateClientConnState(state balancer.ClientConnState) error {
    // ...
    balToUpdate := gsb.latestBalancer()
    // ...
    return balToUpdate.UpdateClientConnState(state)
}
func (b *baseBalancer) UpdateClientConnState(s balancer.ClientConnState) error {
    // ...
    addrsSet := resolver.NewAddressMap()
    // 更新服务对应的 endpoint 信息，存放到 baseBalancer.subConns 当中
    for _, a := range s.ResolverState.Addresses {
        addrsSet.Set(a, nil)
        if _, ok := b.subConns.Get(a); !ok {
            // a is a new address (not existing in b.subConns).
            sc, err := b.cc.NewSubConn([]resolver.Address{a}, balancer.NewSubConnOptions{HealthCheckEnabled: b.config.HealthCheck})
            // ...
            b.subConns.Set(a, sc)
            b.scStates[sc] = connectivity.Idle
            // ...
            sc.Connect()
        }
    }
    for _, a := range b.subConns.Keys() {
        sci, _ := b.subConns.Get(a)
        sc := sci.(balancer.SubConn)
        // a was removed by resolver.
        if _, ok := addrsSet.Get(a); !ok {
            b.cc.RemoveSubConn(sc)
            b.subConns.Delete(a)
        }
    }
    // ...
    // 将 baseBalancer.subConns 注入到 picker 当中
    b.regeneratePicker()
    b.cc.UpdateState(balancer.State{ConnectivityState: b.state, Picker: b.picker})
    return nil
}
func (b *baseBalancer) regeneratePicker() {
    // ...
    readySCs := make(map[balancer.SubConn]SubConnInfo)


    // Filter out all ready SCs from full subConn map.
    for _, addr := range b.subConns.Keys() {
        sci, _ := b.subConns.Get(addr)
        sc := sci.(balancer.SubConn)
        if st, ok := b.scStates[sc]; ok && st == connectivity.Ready {
            readySCs[sc] = SubConnInfo{Address: addr}
        }
    }
    
    // 基于当前最新的 endpoint 信息构建 picker
    b.picker = b.pickerBuilder.Build(PickerBuildInfo{ReadySCs: readySCs})
}
func (*rrPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
    // ...
    scs := make([]balancer.SubConn, 0, len(info.ReadySCs))
    for sc := range info.ReadySCs {
        scs = append(scs, sc)
    }
    return &rrPicker{
        subConns: scs,
        // ...
        next: uint32(grpcrand.Intn(len(scs))),
    }
}
```

## 3.9 grpc 客户端请求

![img](https://pic4.zhimg.com/v2-3481b3ca10bdf19becd23c545385c823_1440w.jpg)



在 grpc 客户端实际发起请求时，会顺延 invoke -> newClientStream -> newClientStreamWithParams -> csAttemp.getTransport -> ClientConn.getTransport 的方法链路进行调用，接着调用 pickerWrapper.pick 方法，获取到其中内置的 rrPicker（round-robin）连接选择器，调用其 Pick 方法进行服务端节点的选择.

```text
func main() {
    // ...


    client := proto.NewHelloServiceClient(conn)
    for {
        resp, _ := client.SayHello(context.Background(), &proto.HelloReq{
            Name: "xiaoxuxiansheng",
        })
        fmt.Printf("resp: %+v", resp)
        // 每隔 1s 发起一轮请求
        <-time.After(time.Second)
    }
}
func (c *helloServiceClient) SayHello(ctx context.Context, in *HelloReq, opts ...grpc.CallOption) (*HelloResp, error) {
    out := new(HelloResp)
    err := c.cc.Invoke(ctx, "/pb.HelloService/SayHello", in, out, opts...)
    // ...
    return out, nil
}
func (cc *ClientConn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...CallOption) error {
    // ...
    return invoke(ctx, method, args, reply, cc, opts...)
}
func invoke(ctx context.Context, method string, req, reply interface{}, cc *ClientConn, opts ...CallOption) error {
    cs, err := newClientStream(ctx, unaryStreamDesc, cc, method, opts...)
    // ...
    if err := cs.SendMsg(req); err != nil {
        return err
    }
    return cs.RecvMsg(reply)
}
func newClientStream(ctx context.Context, desc *StreamDesc, cc *ClientConn, method string, opts ...CallOption) (_ ClientStream, err error) {
    // ...
    var newStream = func(ctx context.Context, done func()) (iresolver.ClientStream, error) {
        return newClientStreamWithParams(ctx, desc, cc, method, mc, onCommit, done, opts...)
    }
    // ...
    return newStream(ctx, func() {})
}
func newClientStreamWithParams(ctx context.Context, desc *StreamDesc, cc *ClientConn, method string, mc serviceconfig.MethodConfig, onCommit, doneFunc func(), opts ...CallOption) (_ iresolver.ClientStream, err error) {
    // ...
    op := func(a *csAttempt) error {
        if err := a.getTransport(); err != nil {
            return err
        }
        if err := a.newStream(); err != nil {
            return err
        }
        // ...
        cs.attempt = a
        return nil
    }
    if err := cs.withRetry(op, func() { cs.bufferForRetryLocked(0, op) }); err != nil {
        return nil, err
    }
    // ...
    return cs, nil
}
func (a *csAttempt) getTransport() error {
    cs := a.cs

    var err error
    a.t, a.pickResult, err = cs.cc.getTransport(a.ctx, cs.callInfo.failFast, cs.callHdr.Method)
    // ...
}
func (cc *ClientConn) getTransport(ctx context.Context, failfast bool, method string) (transport.ClientTransport, balancer.PickResult, error) {
    return cc.blockingpicker.pick(ctx, failfast, balancer.PickInfo{
        Ctx:            ctx,
        FullMethodName: method,
    })
}
func (pw *pickerWrapper) pick(ctx context.Context, failfast bool, info balancer.PickInfo) (transport.ClientTransport, balancer.PickResult, error) {
    var ch chan struct{}


    var lastPickErr error
    for {
        // ...

        ch = pw.blockingCh
        p := pw.picker
        pw.mu.Unlock()

        pickResult, err := p.Pick(info)
        // ...

        acw, ok := pickResult.SubConn.(*acBalancerWrapper)
        // ...
        if t := acw.getAddrConn().getReadyTransport(); t != nil {
            // ...
            return t, pickResult, nil
        }
        // ...
 }
```

## 3.10 roundrobin 负载均衡

grpc-go 中，对 round-robin picker 的实现源码如下，其中包含了两个核心字段：

- 最新的 endpoints 连接列表：subConns
- 上一次获取的连接索引：next

```text
// ...
type rrPicker struct {
    // ...
    // subconn 列表
    subConns []balancer.SubConn
    // 最后一次获取 subconn 时对应的 index
    next     uint32
}
```

每次调用 rrPicker.Pick 方法，会对 next 的数值进行加一，然后取 next 对 endpoints 连接列表 subConnss 取模，获取到对应的一笔 subConn 进行返回，以达到负载均衡的效果.

```text
func (p *rrPicker) Pick(balancer.PickInfo) (balancer.PickResult, error) {
    subConnsLen := uint32(len(p.subConns))
    // 更新 next
    nextIndex := atomic.AddUint32(&p.next, 1)

    // 轮流依次取 subconn
    sc := p.subConns[nextIndex%subConnsLen]
    return balancer.PickResult{SubConn: sc}, nil
}
```

## 4 总结

本文以 etcd 作为 grpc 的服务注册/发现模块，round-robin 作为负载均衡策略，以 grpc 客户端的运行链路为主线，进行了原理分析和源码走读：

- grpc 客户端启动过程中会依次构造 balancer 和 resolver，并开启守护协程对状态变更事件进行监听和处理
- 构造 etcd resolver 并开启守护协程 watcher 进行服务端 endpoints 更新事件的监听
- 负载均衡器 balancer 的守护协程会接收到来自 resolver 传递的 endpoints 列表，对本地缓存的数据进行更新
- grpc 客户端发起请求时，会通过 round-robin 算法，对负载均衡器中缓存的 endpoints 轮询使用