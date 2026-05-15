package com.yaghan.control_plane.v1alpha1;

import static io.grpc.MethodDescriptor.generateFullMethodName;

/**
 */
@io.grpc.stub.annotations.GrpcGenerated
public final class ClusterServiceGrpc {

  private ClusterServiceGrpc() {}

  public static final java.lang.String SERVICE_NAME = "yaghan.control_plane.v1alpha1.ClusterService";

  // Static method descriptors that strictly reflect the proto.
  private static volatile io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.EstablishSessionRequest,
      com.yaghan.control_plane.v1alpha1.EstablishSessionResponse> getEstablishSessionMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "EstablishSession",
      requestType = com.yaghan.control_plane.v1alpha1.EstablishSessionRequest.class,
      responseType = com.yaghan.control_plane.v1alpha1.EstablishSessionResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.BIDI_STREAMING)
  public static io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.EstablishSessionRequest,
      com.yaghan.control_plane.v1alpha1.EstablishSessionResponse> getEstablishSessionMethod() {
    io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.EstablishSessionRequest, com.yaghan.control_plane.v1alpha1.EstablishSessionResponse> getEstablishSessionMethod;
    if ((getEstablishSessionMethod = ClusterServiceGrpc.getEstablishSessionMethod) == null) {
      synchronized (ClusterServiceGrpc.class) {
        if ((getEstablishSessionMethod = ClusterServiceGrpc.getEstablishSessionMethod) == null) {
          ClusterServiceGrpc.getEstablishSessionMethod = getEstablishSessionMethod =
              io.grpc.MethodDescriptor.<com.yaghan.control_plane.v1alpha1.EstablishSessionRequest, com.yaghan.control_plane.v1alpha1.EstablishSessionResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.BIDI_STREAMING)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "EstablishSession"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.control_plane.v1alpha1.EstablishSessionRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.control_plane.v1alpha1.EstablishSessionResponse.getDefaultInstance()))
              .setSchemaDescriptor(new ClusterServiceMethodDescriptorSupplier("EstablishSession"))
              .build();
        }
      }
    }
    return getEstablishSessionMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.GetNodeRequest,
      com.yaghan.control_plane.v1alpha1.GetNodeResponse> getGetNodeMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "GetNode",
      requestType = com.yaghan.control_plane.v1alpha1.GetNodeRequest.class,
      responseType = com.yaghan.control_plane.v1alpha1.GetNodeResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.GetNodeRequest,
      com.yaghan.control_plane.v1alpha1.GetNodeResponse> getGetNodeMethod() {
    io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.GetNodeRequest, com.yaghan.control_plane.v1alpha1.GetNodeResponse> getGetNodeMethod;
    if ((getGetNodeMethod = ClusterServiceGrpc.getGetNodeMethod) == null) {
      synchronized (ClusterServiceGrpc.class) {
        if ((getGetNodeMethod = ClusterServiceGrpc.getGetNodeMethod) == null) {
          ClusterServiceGrpc.getGetNodeMethod = getGetNodeMethod =
              io.grpc.MethodDescriptor.<com.yaghan.control_plane.v1alpha1.GetNodeRequest, com.yaghan.control_plane.v1alpha1.GetNodeResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "GetNode"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.control_plane.v1alpha1.GetNodeRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.control_plane.v1alpha1.GetNodeResponse.getDefaultInstance()))
              .setSchemaDescriptor(new ClusterServiceMethodDescriptorSupplier("GetNode"))
              .build();
        }
      }
    }
    return getGetNodeMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.ListNodesRequest,
      com.yaghan.control_plane.v1alpha1.ListNodesResponse> getListNodesMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "ListNodes",
      requestType = com.yaghan.control_plane.v1alpha1.ListNodesRequest.class,
      responseType = com.yaghan.control_plane.v1alpha1.ListNodesResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.ListNodesRequest,
      com.yaghan.control_plane.v1alpha1.ListNodesResponse> getListNodesMethod() {
    io.grpc.MethodDescriptor<com.yaghan.control_plane.v1alpha1.ListNodesRequest, com.yaghan.control_plane.v1alpha1.ListNodesResponse> getListNodesMethod;
    if ((getListNodesMethod = ClusterServiceGrpc.getListNodesMethod) == null) {
      synchronized (ClusterServiceGrpc.class) {
        if ((getListNodesMethod = ClusterServiceGrpc.getListNodesMethod) == null) {
          ClusterServiceGrpc.getListNodesMethod = getListNodesMethod =
              io.grpc.MethodDescriptor.<com.yaghan.control_plane.v1alpha1.ListNodesRequest, com.yaghan.control_plane.v1alpha1.ListNodesResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "ListNodes"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.control_plane.v1alpha1.ListNodesRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.control_plane.v1alpha1.ListNodesResponse.getDefaultInstance()))
              .setSchemaDescriptor(new ClusterServiceMethodDescriptorSupplier("ListNodes"))
              .build();
        }
      }
    }
    return getListNodesMethod;
  }

  /**
   * Creates a new async stub that supports all call types for the service
   */
  public static ClusterServiceStub newStub(io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<ClusterServiceStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<ClusterServiceStub>() {
        @java.lang.Override
        public ClusterServiceStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new ClusterServiceStub(channel, callOptions);
        }
      };
    return ClusterServiceStub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports all types of calls on the service
   */
  public static ClusterServiceBlockingV2Stub newBlockingV2Stub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<ClusterServiceBlockingV2Stub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<ClusterServiceBlockingV2Stub>() {
        @java.lang.Override
        public ClusterServiceBlockingV2Stub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new ClusterServiceBlockingV2Stub(channel, callOptions);
        }
      };
    return ClusterServiceBlockingV2Stub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports unary and streaming output calls on the service
   */
  public static ClusterServiceBlockingStub newBlockingStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<ClusterServiceBlockingStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<ClusterServiceBlockingStub>() {
        @java.lang.Override
        public ClusterServiceBlockingStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new ClusterServiceBlockingStub(channel, callOptions);
        }
      };
    return ClusterServiceBlockingStub.newStub(factory, channel);
  }

  /**
   * Creates a new ListenableFuture-style stub that supports unary calls on the service
   */
  public static ClusterServiceFutureStub newFutureStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<ClusterServiceFutureStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<ClusterServiceFutureStub>() {
        @java.lang.Override
        public ClusterServiceFutureStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new ClusterServiceFutureStub(channel, callOptions);
        }
      };
    return ClusterServiceFutureStub.newStub(factory, channel);
  }

  /**
   */
  public interface AsyncService {

    /**
     */
    default io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.EstablishSessionRequest> establishSession(
        io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.EstablishSessionResponse> responseObserver) {
      return io.grpc.stub.ServerCalls.asyncUnimplementedStreamingCall(getEstablishSessionMethod(), responseObserver);
    }

    /**
     */
    default void getNode(com.yaghan.control_plane.v1alpha1.GetNodeRequest request,
        io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.GetNodeResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getGetNodeMethod(), responseObserver);
    }

    /**
     */
    default void listNodes(com.yaghan.control_plane.v1alpha1.ListNodesRequest request,
        io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.ListNodesResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getListNodesMethod(), responseObserver);
    }
  }

  /**
   * Base class for the server implementation of the service ClusterService.
   */
  public static abstract class ClusterServiceImplBase
      implements io.grpc.BindableService, AsyncService {

    @java.lang.Override public final io.grpc.ServerServiceDefinition bindService() {
      return ClusterServiceGrpc.bindService(this);
    }
  }

  /**
   * A stub to allow clients to do asynchronous rpc calls to service ClusterService.
   */
  public static final class ClusterServiceStub
      extends io.grpc.stub.AbstractAsyncStub<ClusterServiceStub> {
    private ClusterServiceStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected ClusterServiceStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new ClusterServiceStub(channel, callOptions);
    }

    /**
     */
    public io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.EstablishSessionRequest> establishSession(
        io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.EstablishSessionResponse> responseObserver) {
      return io.grpc.stub.ClientCalls.asyncBidiStreamingCall(
          getChannel().newCall(getEstablishSessionMethod(), getCallOptions()), responseObserver);
    }

    /**
     */
    public void getNode(com.yaghan.control_plane.v1alpha1.GetNodeRequest request,
        io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.GetNodeResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getGetNodeMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void listNodes(com.yaghan.control_plane.v1alpha1.ListNodesRequest request,
        io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.ListNodesResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getListNodesMethod(), getCallOptions()), request, responseObserver);
    }
  }

  /**
   * A stub to allow clients to do synchronous rpc calls to service ClusterService.
   */
  public static final class ClusterServiceBlockingV2Stub
      extends io.grpc.stub.AbstractBlockingStub<ClusterServiceBlockingV2Stub> {
    private ClusterServiceBlockingV2Stub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected ClusterServiceBlockingV2Stub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new ClusterServiceBlockingV2Stub(channel, callOptions);
    }

    /**
     */
    @io.grpc.ExperimentalApi("https://github.com/grpc/grpc-java/issues/10918")
    public io.grpc.stub.BlockingClientCall<com.yaghan.control_plane.v1alpha1.EstablishSessionRequest, com.yaghan.control_plane.v1alpha1.EstablishSessionResponse>
        establishSession() {
      return io.grpc.stub.ClientCalls.blockingBidiStreamingCall(
          getChannel(), getEstablishSessionMethod(), getCallOptions());
    }

    /**
     */
    public com.yaghan.control_plane.v1alpha1.GetNodeResponse getNode(com.yaghan.control_plane.v1alpha1.GetNodeRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getGetNodeMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.yaghan.control_plane.v1alpha1.ListNodesResponse listNodes(com.yaghan.control_plane.v1alpha1.ListNodesRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getListNodesMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do limited synchronous rpc calls to service ClusterService.
   */
  public static final class ClusterServiceBlockingStub
      extends io.grpc.stub.AbstractBlockingStub<ClusterServiceBlockingStub> {
    private ClusterServiceBlockingStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected ClusterServiceBlockingStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new ClusterServiceBlockingStub(channel, callOptions);
    }

    /**
     */
    public com.yaghan.control_plane.v1alpha1.GetNodeResponse getNode(com.yaghan.control_plane.v1alpha1.GetNodeRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getGetNodeMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.yaghan.control_plane.v1alpha1.ListNodesResponse listNodes(com.yaghan.control_plane.v1alpha1.ListNodesRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getListNodesMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do ListenableFuture-style rpc calls to service ClusterService.
   */
  public static final class ClusterServiceFutureStub
      extends io.grpc.stub.AbstractFutureStub<ClusterServiceFutureStub> {
    private ClusterServiceFutureStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected ClusterServiceFutureStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new ClusterServiceFutureStub(channel, callOptions);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.yaghan.control_plane.v1alpha1.GetNodeResponse> getNode(
        com.yaghan.control_plane.v1alpha1.GetNodeRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getGetNodeMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.yaghan.control_plane.v1alpha1.ListNodesResponse> listNodes(
        com.yaghan.control_plane.v1alpha1.ListNodesRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getListNodesMethod(), getCallOptions()), request);
    }
  }

  private static final int METHODID_GET_NODE = 0;
  private static final int METHODID_LIST_NODES = 1;
  private static final int METHODID_ESTABLISH_SESSION = 2;

  private static final class MethodHandlers<Req, Resp> implements
      io.grpc.stub.ServerCalls.UnaryMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.ServerStreamingMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.ClientStreamingMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.BidiStreamingMethod<Req, Resp> {
    private final AsyncService serviceImpl;
    private final int methodId;

    MethodHandlers(AsyncService serviceImpl, int methodId) {
      this.serviceImpl = serviceImpl;
      this.methodId = methodId;
    }

    @java.lang.Override
    @java.lang.SuppressWarnings("unchecked")
    public void invoke(Req request, io.grpc.stub.StreamObserver<Resp> responseObserver) {
      switch (methodId) {
        case METHODID_GET_NODE:
          serviceImpl.getNode((com.yaghan.control_plane.v1alpha1.GetNodeRequest) request,
              (io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.GetNodeResponse>) responseObserver);
          break;
        case METHODID_LIST_NODES:
          serviceImpl.listNodes((com.yaghan.control_plane.v1alpha1.ListNodesRequest) request,
              (io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.ListNodesResponse>) responseObserver);
          break;
        default:
          throw new AssertionError();
      }
    }

    @java.lang.Override
    @java.lang.SuppressWarnings("unchecked")
    public io.grpc.stub.StreamObserver<Req> invoke(
        io.grpc.stub.StreamObserver<Resp> responseObserver) {
      switch (methodId) {
        case METHODID_ESTABLISH_SESSION:
          return (io.grpc.stub.StreamObserver<Req>) serviceImpl.establishSession(
              (io.grpc.stub.StreamObserver<com.yaghan.control_plane.v1alpha1.EstablishSessionResponse>) responseObserver);
        default:
          throw new AssertionError();
      }
    }
  }

  public static final io.grpc.ServerServiceDefinition bindService(AsyncService service) {
    return io.grpc.ServerServiceDefinition.builder(getServiceDescriptor())
        .addMethod(
          getEstablishSessionMethod(),
          io.grpc.stub.ServerCalls.asyncBidiStreamingCall(
            new MethodHandlers<
              com.yaghan.control_plane.v1alpha1.EstablishSessionRequest,
              com.yaghan.control_plane.v1alpha1.EstablishSessionResponse>(
                service, METHODID_ESTABLISH_SESSION)))
        .addMethod(
          getGetNodeMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.yaghan.control_plane.v1alpha1.GetNodeRequest,
              com.yaghan.control_plane.v1alpha1.GetNodeResponse>(
                service, METHODID_GET_NODE)))
        .addMethod(
          getListNodesMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.yaghan.control_plane.v1alpha1.ListNodesRequest,
              com.yaghan.control_plane.v1alpha1.ListNodesResponse>(
                service, METHODID_LIST_NODES)))
        .build();
  }

  private static abstract class ClusterServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoFileDescriptorSupplier, io.grpc.protobuf.ProtoServiceDescriptorSupplier {
    ClusterServiceBaseDescriptorSupplier() {}

    @java.lang.Override
    public com.google.protobuf.Descriptors.FileDescriptor getFileDescriptor() {
      return com.yaghan.control_plane.v1alpha1.ClusterProto.getDescriptor();
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.ServiceDescriptor getServiceDescriptor() {
      return getFileDescriptor().findServiceByName("ClusterService");
    }
  }

  private static final class ClusterServiceFileDescriptorSupplier
      extends ClusterServiceBaseDescriptorSupplier {
    ClusterServiceFileDescriptorSupplier() {}
  }

  private static final class ClusterServiceMethodDescriptorSupplier
      extends ClusterServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoMethodDescriptorSupplier {
    private final java.lang.String methodName;

    ClusterServiceMethodDescriptorSupplier(java.lang.String methodName) {
      this.methodName = methodName;
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.MethodDescriptor getMethodDescriptor() {
      return getServiceDescriptor().findMethodByName(methodName);
    }
  }

  private static volatile io.grpc.ServiceDescriptor serviceDescriptor;

  public static io.grpc.ServiceDescriptor getServiceDescriptor() {
    io.grpc.ServiceDescriptor result = serviceDescriptor;
    if (result == null) {
      synchronized (ClusterServiceGrpc.class) {
        result = serviceDescriptor;
        if (result == null) {
          serviceDescriptor = result = io.grpc.ServiceDescriptor.newBuilder(SERVICE_NAME)
              .setSchemaDescriptor(new ClusterServiceFileDescriptorSupplier())
              .addMethod(getEstablishSessionMethod())
              .addMethod(getGetNodeMethod())
              .addMethod(getListNodesMethod())
              .build();
        }
      }
    }
    return result;
  }
}
