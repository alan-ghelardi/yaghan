package com.nuinfra.control_plane.v1alpha1;

import static io.grpc.MethodDescriptor.generateFullMethodName;

/**
 */
@io.grpc.stub.annotations.GrpcGenerated
public final class SandboxServiceGrpc {

  private SandboxServiceGrpc() {}

  public static final java.lang.String SERVICE_NAME = "nuinfra.control_plane.v1alpha1.SandboxService";

  // Static method descriptors that strictly reflect the proto.
  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse> getCreateSandboxMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "CreateSandbox",
      requestType = com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse> getCreateSandboxMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest, com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse> getCreateSandboxMethod;
    if ((getCreateSandboxMethod = SandboxServiceGrpc.getCreateSandboxMethod) == null) {
      synchronized (SandboxServiceGrpc.class) {
        if ((getCreateSandboxMethod = SandboxServiceGrpc.getCreateSandboxMethod) == null) {
          SandboxServiceGrpc.getCreateSandboxMethod = getCreateSandboxMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest, com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "CreateSandbox"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SandboxServiceMethodDescriptorSupplier("CreateSandbox"))
              .build();
        }
      }
    }
    return getCreateSandboxMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.GetSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.GetSandboxResponse> getGetSandboxMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "GetSandbox",
      requestType = com.nuinfra.control_plane.v1alpha1.GetSandboxRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.GetSandboxResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.GetSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.GetSandboxResponse> getGetSandboxMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.GetSandboxRequest, com.nuinfra.control_plane.v1alpha1.GetSandboxResponse> getGetSandboxMethod;
    if ((getGetSandboxMethod = SandboxServiceGrpc.getGetSandboxMethod) == null) {
      synchronized (SandboxServiceGrpc.class) {
        if ((getGetSandboxMethod = SandboxServiceGrpc.getGetSandboxMethod) == null) {
          SandboxServiceGrpc.getGetSandboxMethod = getGetSandboxMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.GetSandboxRequest, com.nuinfra.control_plane.v1alpha1.GetSandboxResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "GetSandbox"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.GetSandboxRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.GetSandboxResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SandboxServiceMethodDescriptorSupplier("GetSandbox"))
              .build();
        }
      }
    }
    return getGetSandboxMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest,
      com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse> getListSandboxesMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "ListSandboxes",
      requestType = com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest,
      com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse> getListSandboxesMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest, com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse> getListSandboxesMethod;
    if ((getListSandboxesMethod = SandboxServiceGrpc.getListSandboxesMethod) == null) {
      synchronized (SandboxServiceGrpc.class) {
        if ((getListSandboxesMethod = SandboxServiceGrpc.getListSandboxesMethod) == null) {
          SandboxServiceGrpc.getListSandboxesMethod = getListSandboxesMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest, com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "ListSandboxes"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SandboxServiceMethodDescriptorSupplier("ListSandboxes"))
              .build();
        }
      }
    }
    return getListSandboxesMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse> getPauseSandboxMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "PauseSandbox",
      requestType = com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse> getPauseSandboxMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest, com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse> getPauseSandboxMethod;
    if ((getPauseSandboxMethod = SandboxServiceGrpc.getPauseSandboxMethod) == null) {
      synchronized (SandboxServiceGrpc.class) {
        if ((getPauseSandboxMethod = SandboxServiceGrpc.getPauseSandboxMethod) == null) {
          SandboxServiceGrpc.getPauseSandboxMethod = getPauseSandboxMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest, com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "PauseSandbox"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SandboxServiceMethodDescriptorSupplier("PauseSandbox"))
              .build();
        }
      }
    }
    return getPauseSandboxMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse> getResumeSandboxMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "ResumeSandbox",
      requestType = com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse> getResumeSandboxMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest, com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse> getResumeSandboxMethod;
    if ((getResumeSandboxMethod = SandboxServiceGrpc.getResumeSandboxMethod) == null) {
      synchronized (SandboxServiceGrpc.class) {
        if ((getResumeSandboxMethod = SandboxServiceGrpc.getResumeSandboxMethod) == null) {
          SandboxServiceGrpc.getResumeSandboxMethod = getResumeSandboxMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest, com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "ResumeSandbox"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SandboxServiceMethodDescriptorSupplier("ResumeSandbox"))
              .build();
        }
      }
    }
    return getResumeSandboxMethod;
  }

  private static volatile io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse> getDeleteSandboxMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "DeleteSandbox",
      requestType = com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest.class,
      responseType = com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest,
      com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse> getDeleteSandboxMethod() {
    io.grpc.MethodDescriptor<com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest, com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse> getDeleteSandboxMethod;
    if ((getDeleteSandboxMethod = SandboxServiceGrpc.getDeleteSandboxMethod) == null) {
      synchronized (SandboxServiceGrpc.class) {
        if ((getDeleteSandboxMethod = SandboxServiceGrpc.getDeleteSandboxMethod) == null) {
          SandboxServiceGrpc.getDeleteSandboxMethod = getDeleteSandboxMethod =
              io.grpc.MethodDescriptor.<com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest, com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "DeleteSandbox"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse.getDefaultInstance()))
              .setSchemaDescriptor(new SandboxServiceMethodDescriptorSupplier("DeleteSandbox"))
              .build();
        }
      }
    }
    return getDeleteSandboxMethod;
  }

  /**
   * Creates a new async stub that supports all call types for the service
   */
  public static SandboxServiceStub newStub(io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<SandboxServiceStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<SandboxServiceStub>() {
        @java.lang.Override
        public SandboxServiceStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new SandboxServiceStub(channel, callOptions);
        }
      };
    return SandboxServiceStub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports all types of calls on the service
   */
  public static SandboxServiceBlockingV2Stub newBlockingV2Stub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<SandboxServiceBlockingV2Stub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<SandboxServiceBlockingV2Stub>() {
        @java.lang.Override
        public SandboxServiceBlockingV2Stub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new SandboxServiceBlockingV2Stub(channel, callOptions);
        }
      };
    return SandboxServiceBlockingV2Stub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports unary and streaming output calls on the service
   */
  public static SandboxServiceBlockingStub newBlockingStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<SandboxServiceBlockingStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<SandboxServiceBlockingStub>() {
        @java.lang.Override
        public SandboxServiceBlockingStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new SandboxServiceBlockingStub(channel, callOptions);
        }
      };
    return SandboxServiceBlockingStub.newStub(factory, channel);
  }

  /**
   * Creates a new ListenableFuture-style stub that supports unary calls on the service
   */
  public static SandboxServiceFutureStub newFutureStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<SandboxServiceFutureStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<SandboxServiceFutureStub>() {
        @java.lang.Override
        public SandboxServiceFutureStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new SandboxServiceFutureStub(channel, callOptions);
        }
      };
    return SandboxServiceFutureStub.newStub(factory, channel);
  }

  /**
   */
  public interface AsyncService {

    /**
     */
    default void createSandbox(com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getCreateSandboxMethod(), responseObserver);
    }

    /**
     */
    default void getSandbox(com.nuinfra.control_plane.v1alpha1.GetSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.GetSandboxResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getGetSandboxMethod(), responseObserver);
    }

    /**
     */
    default void listSandboxes(com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getListSandboxesMethod(), responseObserver);
    }

    /**
     */
    default void pauseSandbox(com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getPauseSandboxMethod(), responseObserver);
    }

    /**
     */
    default void resumeSandbox(com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getResumeSandboxMethod(), responseObserver);
    }

    /**
     */
    default void deleteSandbox(com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getDeleteSandboxMethod(), responseObserver);
    }
  }

  /**
   * Base class for the server implementation of the service SandboxService.
   */
  public static abstract class SandboxServiceImplBase
      implements io.grpc.BindableService, AsyncService {

    @java.lang.Override public final io.grpc.ServerServiceDefinition bindService() {
      return SandboxServiceGrpc.bindService(this);
    }
  }

  /**
   * A stub to allow clients to do asynchronous rpc calls to service SandboxService.
   */
  public static final class SandboxServiceStub
      extends io.grpc.stub.AbstractAsyncStub<SandboxServiceStub> {
    private SandboxServiceStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected SandboxServiceStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new SandboxServiceStub(channel, callOptions);
    }

    /**
     */
    public void createSandbox(com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getCreateSandboxMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void getSandbox(com.nuinfra.control_plane.v1alpha1.GetSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.GetSandboxResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getGetSandboxMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void listSandboxes(com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getListSandboxesMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void pauseSandbox(com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getPauseSandboxMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void resumeSandbox(com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getResumeSandboxMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void deleteSandbox(com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest request,
        io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getDeleteSandboxMethod(), getCallOptions()), request, responseObserver);
    }
  }

  /**
   * A stub to allow clients to do synchronous rpc calls to service SandboxService.
   */
  public static final class SandboxServiceBlockingV2Stub
      extends io.grpc.stub.AbstractBlockingStub<SandboxServiceBlockingV2Stub> {
    private SandboxServiceBlockingV2Stub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected SandboxServiceBlockingV2Stub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new SandboxServiceBlockingV2Stub(channel, callOptions);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse createSandbox(com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getCreateSandboxMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.GetSandboxResponse getSandbox(com.nuinfra.control_plane.v1alpha1.GetSandboxRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getGetSandboxMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse listSandboxes(com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getListSandboxesMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse pauseSandbox(com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getPauseSandboxMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse resumeSandbox(com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getResumeSandboxMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse deleteSandbox(com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getDeleteSandboxMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do limited synchronous rpc calls to service SandboxService.
   */
  public static final class SandboxServiceBlockingStub
      extends io.grpc.stub.AbstractBlockingStub<SandboxServiceBlockingStub> {
    private SandboxServiceBlockingStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected SandboxServiceBlockingStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new SandboxServiceBlockingStub(channel, callOptions);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse createSandbox(com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getCreateSandboxMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.GetSandboxResponse getSandbox(com.nuinfra.control_plane.v1alpha1.GetSandboxRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getGetSandboxMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse listSandboxes(com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getListSandboxesMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse pauseSandbox(com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getPauseSandboxMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse resumeSandbox(com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getResumeSandboxMethod(), getCallOptions(), request);
    }

    /**
     */
    public com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse deleteSandbox(com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getDeleteSandboxMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do ListenableFuture-style rpc calls to service SandboxService.
   */
  public static final class SandboxServiceFutureStub
      extends io.grpc.stub.AbstractFutureStub<SandboxServiceFutureStub> {
    private SandboxServiceFutureStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected SandboxServiceFutureStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new SandboxServiceFutureStub(channel, callOptions);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse> createSandbox(
        com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getCreateSandboxMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.GetSandboxResponse> getSandbox(
        com.nuinfra.control_plane.v1alpha1.GetSandboxRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getGetSandboxMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse> listSandboxes(
        com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getListSandboxesMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse> pauseSandbox(
        com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getPauseSandboxMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse> resumeSandbox(
        com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getResumeSandboxMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse> deleteSandbox(
        com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getDeleteSandboxMethod(), getCallOptions()), request);
    }
  }

  private static final int METHODID_CREATE_SANDBOX = 0;
  private static final int METHODID_GET_SANDBOX = 1;
  private static final int METHODID_LIST_SANDBOXES = 2;
  private static final int METHODID_PAUSE_SANDBOX = 3;
  private static final int METHODID_RESUME_SANDBOX = 4;
  private static final int METHODID_DELETE_SANDBOX = 5;

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
        case METHODID_CREATE_SANDBOX:
          serviceImpl.createSandbox((com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse>) responseObserver);
          break;
        case METHODID_GET_SANDBOX:
          serviceImpl.getSandbox((com.nuinfra.control_plane.v1alpha1.GetSandboxRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.GetSandboxResponse>) responseObserver);
          break;
        case METHODID_LIST_SANDBOXES:
          serviceImpl.listSandboxes((com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse>) responseObserver);
          break;
        case METHODID_PAUSE_SANDBOX:
          serviceImpl.pauseSandbox((com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse>) responseObserver);
          break;
        case METHODID_RESUME_SANDBOX:
          serviceImpl.resumeSandbox((com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse>) responseObserver);
          break;
        case METHODID_DELETE_SANDBOX:
          serviceImpl.deleteSandbox((com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest) request,
              (io.grpc.stub.StreamObserver<com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse>) responseObserver);
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
          getCreateSandboxMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.CreateSandboxRequest,
              com.nuinfra.control_plane.v1alpha1.CreateSandboxResponse>(
                service, METHODID_CREATE_SANDBOX)))
        .addMethod(
          getGetSandboxMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.GetSandboxRequest,
              com.nuinfra.control_plane.v1alpha1.GetSandboxResponse>(
                service, METHODID_GET_SANDBOX)))
        .addMethod(
          getListSandboxesMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.ListSandboxesRequest,
              com.nuinfra.control_plane.v1alpha1.ListSandboxesResponse>(
                service, METHODID_LIST_SANDBOXES)))
        .addMethod(
          getPauseSandboxMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.PauseSandboxRequest,
              com.nuinfra.control_plane.v1alpha1.PauseSandboxResponse>(
                service, METHODID_PAUSE_SANDBOX)))
        .addMethod(
          getResumeSandboxMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.ResumeSandboxRequest,
              com.nuinfra.control_plane.v1alpha1.ResumeSandboxResponse>(
                service, METHODID_RESUME_SANDBOX)))
        .addMethod(
          getDeleteSandboxMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              com.nuinfra.control_plane.v1alpha1.DeleteSandboxRequest,
              com.nuinfra.control_plane.v1alpha1.DeleteSandboxResponse>(
                service, METHODID_DELETE_SANDBOX)))
        .build();
  }

  private static abstract class SandboxServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoFileDescriptorSupplier, io.grpc.protobuf.ProtoServiceDescriptorSupplier {
    SandboxServiceBaseDescriptorSupplier() {}

    @java.lang.Override
    public com.google.protobuf.Descriptors.FileDescriptor getFileDescriptor() {
      return com.nuinfra.control_plane.v1alpha1.SandboxProto.getDescriptor();
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.ServiceDescriptor getServiceDescriptor() {
      return getFileDescriptor().findServiceByName("SandboxService");
    }
  }

  private static final class SandboxServiceFileDescriptorSupplier
      extends SandboxServiceBaseDescriptorSupplier {
    SandboxServiceFileDescriptorSupplier() {}
  }

  private static final class SandboxServiceMethodDescriptorSupplier
      extends SandboxServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoMethodDescriptorSupplier {
    private final java.lang.String methodName;

    SandboxServiceMethodDescriptorSupplier(java.lang.String methodName) {
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
      synchronized (SandboxServiceGrpc.class) {
        result = serviceDescriptor;
        if (result == null) {
          serviceDescriptor = result = io.grpc.ServiceDescriptor.newBuilder(SERVICE_NAME)
              .setSchemaDescriptor(new SandboxServiceFileDescriptorSupplier())
              .addMethod(getCreateSandboxMethod())
              .addMethod(getGetSandboxMethod())
              .addMethod(getListSandboxesMethod())
              .addMethod(getPauseSandboxMethod())
              .addMethod(getResumeSandboxMethod())
              .addMethod(getDeleteSandboxMethod())
              .build();
        }
      }
    }
    return result;
  }
}
