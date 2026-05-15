package com.yaghan.data_plane.v1alpha1;

import static io.grpc.MethodDescriptor.generateFullMethodName;

/**
 */
@io.grpc.stub.annotations.GrpcGenerated
public final class DaemonServiceGrpc {

  private DaemonServiceGrpc() {}

  public static final java.lang.String SERVICE_NAME = "yaghan.data_plane.v1alpha1.DaemonService";

  // Static method descriptors that strictly reflect the proto.
  private static volatile io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.ExecRequest,
      com.yaghan.data_plane.v1alpha1.ExecResponse> getExecMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "Exec",
      requestType = com.yaghan.data_plane.v1alpha1.ExecRequest.class,
      responseType = com.yaghan.data_plane.v1alpha1.ExecResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.BIDI_STREAMING)
  public static io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.ExecRequest,
      com.yaghan.data_plane.v1alpha1.ExecResponse> getExecMethod() {
    io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.ExecRequest, com.yaghan.data_plane.v1alpha1.ExecResponse> getExecMethod;
    if ((getExecMethod = DaemonServiceGrpc.getExecMethod) == null) {
      synchronized (DaemonServiceGrpc.class) {
        if ((getExecMethod = DaemonServiceGrpc.getExecMethod) == null) {
          DaemonServiceGrpc.getExecMethod = getExecMethod =
              io.grpc.MethodDescriptor.<com.yaghan.data_plane.v1alpha1.ExecRequest, com.yaghan.data_plane.v1alpha1.ExecResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.BIDI_STREAMING)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "Exec"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.data_plane.v1alpha1.ExecRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.data_plane.v1alpha1.ExecResponse.getDefaultInstance()))
              .setSchemaDescriptor(new DaemonServiceMethodDescriptorSupplier("Exec"))
              .build();
        }
      }
    }
    return getExecMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.UploadFileRequest,
      com.yaghan.data_plane.v1alpha1.UploadFileResponse> getUploadFileMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "UploadFile",
      requestType = com.yaghan.data_plane.v1alpha1.UploadFileRequest.class,
      responseType = com.yaghan.data_plane.v1alpha1.UploadFileResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.UploadFileRequest,
      com.yaghan.data_plane.v1alpha1.UploadFileResponse> getUploadFileMethod() {
    io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.UploadFileRequest, com.yaghan.data_plane.v1alpha1.UploadFileResponse> getUploadFileMethod;
    if ((getUploadFileMethod = DaemonServiceGrpc.getUploadFileMethod) == null) {
      synchronized (DaemonServiceGrpc.class) {
        if ((getUploadFileMethod = DaemonServiceGrpc.getUploadFileMethod) == null) {
          DaemonServiceGrpc.getUploadFileMethod = getUploadFileMethod =
              io.grpc.MethodDescriptor.<com.yaghan.data_plane.v1alpha1.UploadFileRequest, com.yaghan.data_plane.v1alpha1.UploadFileResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "UploadFile"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.data_plane.v1alpha1.UploadFileRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.data_plane.v1alpha1.UploadFileResponse.getDefaultInstance()))
              .setSchemaDescriptor(new DaemonServiceMethodDescriptorSupplier("UploadFile"))
              .build();
        }
      }
    }
    return getUploadFileMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.DownloadFileRequest,
      com.yaghan.data_plane.v1alpha1.DownloadFileResponse> getDownloadFileMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "DownloadFile",
      requestType = com.yaghan.data_plane.v1alpha1.DownloadFileRequest.class,
      responseType = com.yaghan.data_plane.v1alpha1.DownloadFileResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.DownloadFileRequest,
      com.yaghan.data_plane.v1alpha1.DownloadFileResponse> getDownloadFileMethod() {
    io.grpc.MethodDescriptor<com.yaghan.data_plane.v1alpha1.DownloadFileRequest, com.yaghan.data_plane.v1alpha1.DownloadFileResponse> getDownloadFileMethod;
    if ((getDownloadFileMethod = DaemonServiceGrpc.getDownloadFileMethod) == null) {
      synchronized (DaemonServiceGrpc.class) {
        if ((getDownloadFileMethod = DaemonServiceGrpc.getDownloadFileMethod) == null) {
          DaemonServiceGrpc.getDownloadFileMethod = getDownloadFileMethod =
              io.grpc.MethodDescriptor.<com.yaghan.data_plane.v1alpha1.DownloadFileRequest, com.yaghan.data_plane.v1alpha1.DownloadFileResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "DownloadFile"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.data_plane.v1alpha1.DownloadFileRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.yaghan.data_plane.v1alpha1.DownloadFileResponse.getDefaultInstance()))
              .setSchemaDescriptor(new DaemonServiceMethodDescriptorSupplier("DownloadFile"))
              .build();
        }
      }
    }
    return getDownloadFileMethod;
  }

  /**
   * Creates a new async stub that supports all call types for the service
   */
  public static DaemonServiceStub newStub(io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<DaemonServiceStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<DaemonServiceStub>() {
        @java.lang.Override
        public DaemonServiceStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new DaemonServiceStub(channel, callOptions);
        }
      };
    return DaemonServiceStub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports all types of calls on the service
   */
  public static DaemonServiceBlockingV2Stub newBlockingV2Stub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<DaemonServiceBlockingV2Stub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<DaemonServiceBlockingV2Stub>() {
        @java.lang.Override
        public DaemonServiceBlockingV2Stub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new DaemonServiceBlockingV2Stub(channel, callOptions);
        }
      };
    return DaemonServiceBlockingV2Stub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports unary and streaming output calls on the service
   */
  public static DaemonServiceBlockingStub newBlockingStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<DaemonServiceBlockingStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<DaemonServiceBlockingStub>() {
        @java.lang.Override
        public DaemonServiceBlockingStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new DaemonServiceBlockingStub(channel, callOptions);
        }
      };
    return DaemonServiceBlockingStub.newStub(factory, channel);
  }

  /**
   * Creates a new ListenableFuture-style stub that supports unary calls on the service
   */
  public static DaemonServiceFutureStub newFutureStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<DaemonServiceFutureStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<DaemonServiceFutureStub>() {
        @java.lang.Override
        public DaemonServiceFutureStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new DaemonServiceFutureStub(channel, callOptions);
        }
      };
    return DaemonServiceFutureStub.newStub(factory, channel);
  }

  /**
   */
  public interface AsyncService {

    /**
     */
    default io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.ExecRequest> exec(
        io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.ExecResponse> responseObserver) {
      return io.grpc.stub.ServerCalls.asyncUnimplementedStreamingCall(getExecMethod(), responseObserver);
    }

    /**
     */
    default void uploadFile(com.yaghan.data_plane.v1alpha1.UploadFileRequest request,
        io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.UploadFileResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getUploadFileMethod(), responseObserver);
    }

    /**
     */
    default void downloadFile(com.yaghan.data_plane.v1alpha1.DownloadFileRequest request,
        io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.DownloadFileResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getDownloadFileMethod(), responseObserver);
    }
  }

  /**
   * Base class for the server implementation of the service DaemonService.
   */
  public static abstract class DaemonServiceImplBase
      implements io.grpc.BindableService, AsyncService {

    @java.lang.Override public final io.grpc.ServerServiceDefinition bindService() {
      return DaemonServiceGrpc.bindService(this);
    }
  }

  /**
   * A stub to allow clients to do asynchronous rpc calls to service DaemonService.
   */
  public static final class DaemonServiceStub
      extends io.grpc.stub.AbstractAsyncStub<DaemonServiceStub> {
    private DaemonServiceStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected DaemonServiceStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new DaemonServiceStub(channel, callOptions);
    }

    /**
     */
    public io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.ExecRequest> exec(
        io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.ExecResponse> responseObserver) {
      return io.grpc.stub.ClientCalls.asyncBidiStreamingCall(
          getChannel().newCall(getExecMethod(), getCallOptions()), responseObserver);
    }

    /**
     */
    public void uploadFile(com.yaghan.data_plane.v1alpha1.UploadFileRequest request,
        io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.UploadFileResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getUploadFileMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void downloadFile(com.yaghan.data_plane.v1alpha1.DownloadFileRequest request,
        io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.DownloadFileResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getDownloadFileMethod(), getCallOptions()), request, responseObserver);
    }
  }

  /**
   * A stub to allow clients to do synchronous rpc calls to service DaemonService.
   */
  public static final class DaemonServiceBlockingV2Stub
      extends io.grpc.stub.AbstractBlockingStub<DaemonServiceBlockingV2Stub> {
    private DaemonServiceBlockingV2Stub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected DaemonServiceBlockingV2Stub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new DaemonServiceBlockingV2Stub(channel, callOptions);
    }

    /**
     */
    @io.grpc.ExperimentalApi("https://github.com/grpc/grpc-java/issues/10918")
    public io.grpc.stub.BlockingClientCall<com.yaghan.data_plane.v1alpha1.ExecRequest, com.yaghan.data_plane.v1alpha1.ExecResponse>
        exec() {
      return io.grpc.stub.ClientCalls.blockingBidiStreamingCall(
          getChannel(), getExecMethod(), getCallOptions());
    }

    /**
     */
    public com.yaghan.data_plane.v1alpha1.UploadFileResponse uploadFile(com.yaghan.data_plane.v1alpha1.UploadFileRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getUploadFileMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.yaghan.data_plane.v1alpha1.DownloadFileResponse downloadFile(com.yaghan.data_plane.v1alpha1.DownloadFileRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getDownloadFileMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do limited synchronous rpc calls to service DaemonService.
   */
  public static final class DaemonServiceBlockingStub
      extends io.grpc.stub.AbstractBlockingStub<DaemonServiceBlockingStub> {
    private DaemonServiceBlockingStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected DaemonServiceBlockingStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new DaemonServiceBlockingStub(channel, callOptions);
    }

    /**
     */
    public com.yaghan.data_plane.v1alpha1.UploadFileResponse uploadFile(com.yaghan.data_plane.v1alpha1.UploadFileRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getUploadFileMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.yaghan.data_plane.v1alpha1.DownloadFileResponse downloadFile(com.yaghan.data_plane.v1alpha1.DownloadFileRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getDownloadFileMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do ListenableFuture-style rpc calls to service DaemonService.
   */
  public static final class DaemonServiceFutureStub
      extends io.grpc.stub.AbstractFutureStub<DaemonServiceFutureStub> {
    private DaemonServiceFutureStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected DaemonServiceFutureStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new DaemonServiceFutureStub(channel, callOptions);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.yaghan.data_plane.v1alpha1.UploadFileResponse> uploadFile(
        com.yaghan.data_plane.v1alpha1.UploadFileRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getUploadFileMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.yaghan.data_plane.v1alpha1.DownloadFileResponse> downloadFile(
        com.yaghan.data_plane.v1alpha1.DownloadFileRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getDownloadFileMethod(), getCallOptions()), request);
    }
  }

  private static final int METHODID_UPLOAD_FILE = 0;
  private static final int METHODID_DOWNLOAD_FILE = 1;
  private static final int METHODID_EXEC = 2;

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
        case METHODID_UPLOAD_FILE:
          serviceImpl.uploadFile((com.yaghan.data_plane.v1alpha1.UploadFileRequest) request,
              (io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.UploadFileResponse>) responseObserver);
          break;
        case METHODID_DOWNLOAD_FILE:
          serviceImpl.downloadFile((com.yaghan.data_plane.v1alpha1.DownloadFileRequest) request,
              (io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.DownloadFileResponse>) responseObserver);
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
        case METHODID_EXEC:
          return (io.grpc.stub.StreamObserver<Req>) serviceImpl.exec(
              (io.grpc.stub.StreamObserver<com.yaghan.data_plane.v1alpha1.ExecResponse>) responseObserver);
        default:
          throw new AssertionError();
      }
    }
  }

  public static final io.grpc.ServerServiceDefinition bindService(AsyncService service) {
    return io.grpc.ServerServiceDefinition.builder(getServiceDescriptor())
        .addMethod(
          getExecMethod(),
          io.grpc.stub.ServerCalls.asyncBidiStreamingCall(
            new MethodHandlers<
              com.yaghan.data_plane.v1alpha1.ExecRequest,
              com.yaghan.data_plane.v1alpha1.ExecResponse>(
                service, METHODID_EXEC)))
        .addMethod(
          getUploadFileMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.yaghan.data_plane.v1alpha1.UploadFileRequest,
              com.yaghan.data_plane.v1alpha1.UploadFileResponse>(
                service, METHODID_UPLOAD_FILE)))
        .addMethod(
          getDownloadFileMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.yaghan.data_plane.v1alpha1.DownloadFileRequest,
              com.yaghan.data_plane.v1alpha1.DownloadFileResponse>(
                service, METHODID_DOWNLOAD_FILE)))
        .build();
  }

  private static abstract class DaemonServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoFileDescriptorSupplier, io.grpc.protobuf.ProtoServiceDescriptorSupplier {
    DaemonServiceBaseDescriptorSupplier() {}

    @java.lang.Override
    public com.google.protobuf.Descriptors.FileDescriptor getFileDescriptor() {
      return com.yaghan.data_plane.v1alpha1.DaemonProto.getDescriptor();
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.ServiceDescriptor getServiceDescriptor() {
      return getFileDescriptor().findServiceByName("DaemonService");
    }
  }

  private static final class DaemonServiceFileDescriptorSupplier
      extends DaemonServiceBaseDescriptorSupplier {
    DaemonServiceFileDescriptorSupplier() {}
  }

  private static final class DaemonServiceMethodDescriptorSupplier
      extends DaemonServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoMethodDescriptorSupplier {
    private final java.lang.String methodName;

    DaemonServiceMethodDescriptorSupplier(java.lang.String methodName) {
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
      synchronized (DaemonServiceGrpc.class) {
        result = serviceDescriptor;
        if (result == null) {
          serviceDescriptor = result = io.grpc.ServiceDescriptor.newBuilder(SERVICE_NAME)
              .setSchemaDescriptor(new DaemonServiceFileDescriptorSupplier())
              .addMethod(getExecMethod())
              .addMethod(getUploadFileMethod())
              .addMethod(getDownloadFileMethod())
              .build();
        }
      }
    }
    return result;
  }
}
