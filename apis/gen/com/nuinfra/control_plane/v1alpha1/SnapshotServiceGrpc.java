package com.nuinfra.control_plane.v1alpha1;

import static io.grpc.MethodDescriptor.generateFullMethodName;

/**
 */
@io.grpc.stub.annotations.GrpcGenerated
public final class SnapshotServiceGrpc {

  private SnapshotServiceGrpc() {}

  public static final java.lang.String SERVICE_NAME = "nuinfra.control_plane.v1alpha1.SnapshotService";

  // Static method descriptors that strictly reflect the proto.
  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest,
      com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse> getCreateSnapshotMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "CreateSnapshot",
      requestType = com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest,
      com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse> getCreateSnapshotMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest, com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse> getCreateSnapshotMethod;
    if ((getCreateSnapshotMethod = SnapshotServiceGrpc.getCreateSnapshotMethod) == null) {
      synchronized (SnapshotServiceGrpc.class) {
        if ((getCreateSnapshotMethod = SnapshotServiceGrpc.getCreateSnapshotMethod) == null) {
          SnapshotServiceGrpc.getCreateSnapshotMethod = getCreateSnapshotMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest, com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "CreateSnapshot"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SnapshotServiceMethodDescriptorSupplier("CreateSnapshot"))
              .build();
        }
      }
    }
    return getCreateSnapshotMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest,
      com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse> getGetSnapshotMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "GetSnapshot",
      requestType = com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest,
      com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse> getGetSnapshotMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest, com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse> getGetSnapshotMethod;
    if ((getGetSnapshotMethod = SnapshotServiceGrpc.getGetSnapshotMethod) == null) {
      synchronized (SnapshotServiceGrpc.class) {
        if ((getGetSnapshotMethod = SnapshotServiceGrpc.getGetSnapshotMethod) == null) {
          SnapshotServiceGrpc.getGetSnapshotMethod = getGetSnapshotMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest, com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "GetSnapshot"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SnapshotServiceMethodDescriptorSupplier("GetSnapshot"))
              .build();
        }
      }
    }
    return getGetSnapshotMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest,
      com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse> getListSnapshotsMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "ListSnapshots",
      requestType = com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest,
      com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse> getListSnapshotsMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest, com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse> getListSnapshotsMethod;
    if ((getListSnapshotsMethod = SnapshotServiceGrpc.getListSnapshotsMethod) == null) {
      synchronized (SnapshotServiceGrpc.class) {
        if ((getListSnapshotsMethod = SnapshotServiceGrpc.getListSnapshotsMethod) == null) {
          SnapshotServiceGrpc.getListSnapshotsMethod = getListSnapshotsMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest, com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "ListSnapshots"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SnapshotServiceMethodDescriptorSupplier("ListSnapshots"))
              .build();
        }
      }
    }
    return getListSnapshotsMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest,
      com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse> getDeleteSnapshotMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "DeleteSnapshot",
      requestType = com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest,
      com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse> getDeleteSnapshotMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest, com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse> getDeleteSnapshotMethod;
    if ((getDeleteSnapshotMethod = SnapshotServiceGrpc.getDeleteSnapshotMethod) == null) {
      synchronized (SnapshotServiceGrpc.class) {
        if ((getDeleteSnapshotMethod = SnapshotServiceGrpc.getDeleteSnapshotMethod) == null) {
          SnapshotServiceGrpc.getDeleteSnapshotMethod = getDeleteSnapshotMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest, com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "DeleteSnapshot"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SnapshotServiceMethodDescriptorSupplier("DeleteSnapshot"))
              .build();
        }
      }
    }
    return getDeleteSnapshotMethod;
  }

  /**
   * Creates a new async stub that supports all call types for the service
   */
  public static SnapshotServiceStub newStub(io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<SnapshotServiceStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<SnapshotServiceStub>() {
        @java.lang.Override
        public SnapshotServiceStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new SnapshotServiceStub(channel, callOptions);
        }
      };
    return SnapshotServiceStub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports all types of calls on the service
   */
  public static SnapshotServiceBlockingV2Stub newBlockingV2Stub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<SnapshotServiceBlockingV2Stub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<SnapshotServiceBlockingV2Stub>() {
        @java.lang.Override
        public SnapshotServiceBlockingV2Stub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new SnapshotServiceBlockingV2Stub(channel, callOptions);
        }
      };
    return SnapshotServiceBlockingV2Stub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports unary and streaming output calls on the service
   */
  public static SnapshotServiceBlockingStub newBlockingStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<SnapshotServiceBlockingStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<SnapshotServiceBlockingStub>() {
        @java.lang.Override
        public SnapshotServiceBlockingStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new SnapshotServiceBlockingStub(channel, callOptions);
        }
      };
    return SnapshotServiceBlockingStub.newStub(factory, channel);
  }

  /**
   * Creates a new ListenableFuture-style stub that supports unary calls on the service
   */
  public static SnapshotServiceFutureStub newFutureStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<SnapshotServiceFutureStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<SnapshotServiceFutureStub>() {
        @java.lang.Override
        public SnapshotServiceFutureStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new SnapshotServiceFutureStub(channel, callOptions);
        }
      };
    return SnapshotServiceFutureStub.newStub(factory, channel);
  }

  /**
   */
  public interface AsyncService {

    /**
     */
    default void createSnapshot(com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getCreateSnapshotMethod(), responseObserver);
    }

    /**
     */
    default void getSnapshot(com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getGetSnapshotMethod(), responseObserver);
    }

    /**
     */
    default void listSnapshots(com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getListSnapshotsMethod(), responseObserver);
    }

    /**
     */
    default void deleteSnapshot(com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getDeleteSnapshotMethod(), responseObserver);
    }
  }

  /**
   * Base class for the server implementation of the service SnapshotService.
   */
  public static abstract class SnapshotServiceImplBase
      implements io.grpc.BindableService, AsyncService {

    @java.lang.Override public final io.grpc.ServerServiceDefinition bindService() {
      return SnapshotServiceGrpc.bindService(this);
    }
  }

  /**
   * A stub to allow clients to do asynchronous rpc calls to service SnapshotService.
   */
  public static final class SnapshotServiceStub
      extends io.grpc.stub.AbstractAsyncStub<SnapshotServiceStub> {
    private SnapshotServiceStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected SnapshotServiceStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new SnapshotServiceStub(channel, callOptions);
    }

    /**
     */
    public void createSnapshot(com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getCreateSnapshotMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void getSnapshot(com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getGetSnapshotMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void listSnapshots(com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getListSnapshotsMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void deleteSnapshot(com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getDeleteSnapshotMethod(), getCallOptions()), request, responseObserver);
    }
  }

  /**
   * A stub to allow clients to do synchronous rpc calls to service SnapshotService.
   */
  public static final class SnapshotServiceBlockingV2Stub
      extends io.grpc.stub.AbstractBlockingStub<SnapshotServiceBlockingV2Stub> {
    private SnapshotServiceBlockingV2Stub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected SnapshotServiceBlockingV2Stub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new SnapshotServiceBlockingV2Stub(channel, callOptions);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse createSnapshot(com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getCreateSnapshotMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse getSnapshot(com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getGetSnapshotMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse listSnapshots(com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getListSnapshotsMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse deleteSnapshot(com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getDeleteSnapshotMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do limited synchronous rpc calls to service SnapshotService.
   */
  public static final class SnapshotServiceBlockingStub
      extends io.grpc.stub.AbstractBlockingStub<SnapshotServiceBlockingStub> {
    private SnapshotServiceBlockingStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected SnapshotServiceBlockingStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new SnapshotServiceBlockingStub(channel, callOptions);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse createSnapshot(com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getCreateSnapshotMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse getSnapshot(com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getGetSnapshotMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse listSnapshots(com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getListSnapshotsMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse deleteSnapshot(com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getDeleteSnapshotMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do ListenableFuture-style rpc calls to service SnapshotService.
   */
  public static final class SnapshotServiceFutureStub
      extends io.grpc.stub.AbstractFutureStub<SnapshotServiceFutureStub> {
    private SnapshotServiceFutureStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected SnapshotServiceFutureStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new SnapshotServiceFutureStub(channel, callOptions);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse> createSnapshot(
        com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getCreateSnapshotMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse> getSnapshot(
        com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getGetSnapshotMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse> listSnapshots(
        com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getListSnapshotsMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse> deleteSnapshot(
        com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getDeleteSnapshotMethod(), getCallOptions()), request);
    }
  }

  private static final int METHODID_CREATE_SNAPSHOT = 0;
  private static final int METHODID_GET_SNAPSHOT = 1;
  private static final int METHODID_LIST_SNAPSHOTS = 2;
  private static final int METHODID_DELETE_SNAPSHOT = 3;

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
        case METHODID_CREATE_SNAPSHOT:
          serviceImpl.createSnapshot((com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse>) responseObserver);
          break;
        case METHODID_GET_SNAPSHOT:
          serviceImpl.getSnapshot((com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse>) responseObserver);
          break;
        case METHODID_LIST_SNAPSHOTS:
          serviceImpl.listSnapshots((com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse>) responseObserver);
          break;
        case METHODID_DELETE_SNAPSHOT:
          serviceImpl.deleteSnapshot((com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse>) responseObserver);
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
        default:
          throw new AssertionError();
      }
    }
  }

  public static final io.grpc.ServerServiceDefinition bindService(AsyncService service) {
    return io.grpc.ServerServiceDefinition.builder(getServiceDescriptor())
        .addMethod(
          getCreateSnapshotMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.CreateSnapshotRequest,
              com.nuinfra.control_plane.v1alpha1.CreateSnapshotResponse>(
                service, METHODID_CREATE_SNAPSHOT)))
        .addMethod(
          getGetSnapshotMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.GetSnapshotRequest,
              com.nuinfra.control_plane.v1alpha1.GetSnapshotResponse>(
                service, METHODID_GET_SNAPSHOT)))
        .addMethod(
          getListSnapshotsMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.ListSnapshotsRequest,
              com.nuinfra.control_plane.v1alpha1.ListSnapshotsResponse>(
                service, METHODID_LIST_SNAPSHOTS)))
        .addMethod(
          getDeleteSnapshotMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.DeleteSnapshotRequest,
              com.nuinfra.control_plane.v1alpha1.DeleteSnapshotResponse>(
                service, METHODID_DELETE_SNAPSHOT)))
        .build();
  }

  private static abstract class SnapshotServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoFileDescriptorSupplier, io.grpc.protobuf.ProtoServiceDescriptorSupplier {
    SnapshotServiceBaseDescriptorSupplier() {}

    @java.lang.Override
    public com.google.protobuf.Descriptors.FileDescriptor getFileDescriptor() {
      return com.nuinfra.control_plane.v1alpha1.SnapshotProto.getDescriptor();
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.ServiceDescriptor getServiceDescriptor() {
      return getFileDescriptor().findServiceByName("SnapshotService");
    }
  }

  private static final class SnapshotServiceFileDescriptorSupplier
      extends SnapshotServiceBaseDescriptorSupplier {
    SnapshotServiceFileDescriptorSupplier() {}
  }

  private static final class SnapshotServiceMethodDescriptorSupplier
      extends SnapshotServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoMethodDescriptorSupplier {
    private final java.lang.String methodName;

    SnapshotServiceMethodDescriptorSupplier(java.lang.String methodName) {
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
      synchronized (SnapshotServiceGrpc.class) {
        result = serviceDescriptor;
        if (result == null) {
          serviceDescriptor = result = io.grpc.ServiceDescriptor.newBuilder(SERVICE_NAME)
              .setSchemaDescriptor(new SnapshotServiceFileDescriptorSupplier())
              .addMethod(getCreateSnapshotMethod())
              .addMethod(getGetSnapshotMethod())
              .addMethod(getListSnapshotsMethod())
              .addMethod(getDeleteSnapshotMethod())
              .build();
        }
      }
    }
    return result;
  }
}
