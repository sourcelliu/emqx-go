%%--------------------------------------------------------------------
%% Copyright (c) 2025 EMQ Technologies Co., Ltd. All Rights Reserved.
%%--------------------------------------------------------------------

-module(emqx_mq_sub).

-moduledoc """
The module represents a subscription to a Message Queue consumer.
It handles interactions between a channel and a consumer.

It has states:
* `#finding_mq{}` - no MQ found, we are waiting for it to be created.
* `#connecting{}` - MQ found, the subscription is trying to connect to the consumer.
* `#connected{}` - the subscription is established and the consumer is connected.

It uses timers:
* In `#finding_mq{}` state:
  * `find_mq_retry` - the timeout for the next retry to find the MQ (if it is not present).
* In `#connecting{}` state:
  * `connect_timeout` - the timeout for the consumer to connect to the MQ.
* In `#connected{}` state:
  * `consumer_timeout` - the timeout for the consumer to report about itself
    (either by ping, message, or connection confirmation).
  * `ping` - the timeout for the ping message to be sent to the consumer.
  * `publish_retry` - the timeout for the next retry to push buffered messages into the session's inflight.
""".

-include("emqx_mq_internal.hrl").
-include_lib("snabbkaffe/include/snabbkaffe.hrl").

-export([
    mq_topic_filter/1,
    subscriber_ref/1,
    inspect/1
]).

-export([
    handle_connect/2,
    handle_ack/3,
    handle_info/2,
    handle_disconnect/1
]).

-export([
    connected/2,
    ping/1,
    messages/3
]).

-export([
    connected_v1/2,
    ping_v1/1,
    messages_v1/3
]).

-record(finding_mq, {
    find_mq_retry_tref :: reference() | undefined
}).
-record(connecting, {
    mq :: emqx_mq_types:mq(),
    connect_timeout_tref :: reference() | undefined
}).
-record(connected, {
    mq :: emqx_mq_types:mq(),
    consumer_ref :: emqx_mq_types:consumer_ref(),
    ping_tref :: reference() | undefined,
    consumer_timeout_tref :: reference() | undefined,
    inflight :: #{emqx_mq_types:message_id() => emqx_types:message()},
    buffer :: emqx_mq_sub_buffer:t(),
    publish_retry_tref :: reference() | undefined
}).

-type status() :: #finding_mq{} | #connecting{} | #connected{}.

-type t() :: #{
    status := status(),
    clientid := emqx_types:clientid(),
    topic_filter := emqx_mq_types:mq_topic(),
    subscriber_ref := emqx_mq_types:subscriber_ref()
}.

-export_type([t/0]).

%%--------------------------------------------------------------------
%% Constants
%%--------------------------------------------------------------------

-define(CONNECT_RETRY_INTERVAL_UNEXPECTED_ERROR, 1000).
%% Quick retry if conflict occurred when trying to connect to the consumer and spawning a new one.
-define(CONNECT_RETRY_INTERVAL_ALREADY_REGISTERED, 100).

%%--------------------------------------------------------------------
%% Messages
%%--------------------------------------------------------------------

-record(find_mq_retry, {}).
-record(ping_consumer, {}).
-record(consumer_connect_timeout, {}).
-record(consumer_timeout, {}).
-record(publish_retry, {}).

%%--------------------------------------------------------------------
%% API
%%--------------------------------------------------------------------

-spec mq_topic_filter(t()) -> emqx_mq_types:mq_topic().
mq_topic_filter(#{topic_filter := TopicFilter}) ->
    TopicFilter.

-spec subscriber_ref(t()) -> emqx_mq_types:subscriber_ref().
subscriber_ref(#{subscriber_ref := SubscriberRef}) ->
    SubscriberRef.

-spec inspect(t()) -> map().
inspect(#{status := Status} = Sub) ->
    Info = maps:with([clientid, topic_filter, subscriber_ref], Sub),
    Info#{status => status_inspect(Status)}.

-spec handle_connect(emqx_types:clientinfo(), emqx_mq_types:mq_topic()) -> t().
handle_connect(#{clientid := ClientId}, MQTopic) ->
    SubscriberRef = alias(),
    Sub = #{
        clientid => ClientId,
        topic_filter => MQTopic,
        subscriber_ref => SubscriberRef
    },
    case emqx_mq_registry:find(MQTopic) of
        not_found ->
            %% No queue found, we will retry to find it later.
            %% NOTE
            %% We may register the subscription finders somewhere
            %% and react on queue creation immediately.
            Status = #finding_mq{
                find_mq_retry_tref = send_after(
                    Sub, find_mq_retry_interval(), #find_mq_retry{}
                )
            },
            Sub#{status => Status};
        {ok, MQ} ->
            case emqx_mq_consumer:connect(MQ, SubscriberRef, ClientId) of
                {error, Reason} ->
                    %% MQ found but something went wrong with the consumer.
                    %% Retry to find the queue later.
                    RetryInterval = connect_retry_interval(Reason),
                    case Reason of
                        R when R =:= already_registered orelse R =:= no_mq ->
                            ?tp_debug(mq_sub_handle_connect_error, #{
                                reason => Reason,
                                mq => MQTopic,
                                subscriber_ref => SubscriberRef,
                                clientid => ClientId,
                                retry_interval => RetryInterval
                            });
                        _ ->
                            ?tp(error, mq_sub_handle_connect_error, #{
                                reason => Reason,
                                mq => MQTopic,
                                subscriber_ref => SubscriberRef,
                                clientid => ClientId,
                                retry_interval => RetryInterval
                            })
                    end,
                    Status = #finding_mq{
                        find_mq_retry_tref = send_after(
                            Sub, RetryInterval, #find_mq_retry{}
                        )
                    },
                    Sub#{status => Status};
                ok ->
                    %% MQ and its consumer found, let's connect and wait for the consumer to be ready.
                    Status = #connecting{
                        mq = MQ,
                        connect_timeout_tref = send_after(
                            Sub, consumer_timeout(MQ), #consumer_connect_timeout{}
                        )
                    },
                    Sub#{status => Status}
            end
    end.

-spec handle_ack(t(), emqx_types:message(), emqx_mq_types:ack()) -> ok.
handle_ack(
    #{status := #connected{}} = Sub, Msg, Ack
) ->
    case emqx_message:get_header(?MQ_HEADER_MESSAGE_ID, Msg) of
        undefined ->
            ok;
        MessageId ->
            {ok, do_handle_ack(Sub, MessageId, Ack)}
    end.

-spec handle_info(t(), term()) -> {ok, t()} | {ok, t(), emqx_types:message()} | {error, term()}.
%%
%% Messages from the consumer
%%
handle_info(Sub, #mq_sub_ping{}) ->
    ?tp_debug(mq_sub_ping, #{sub => inspect(Sub)}),
    {ok, reset_consumer_timeout_timer(Sub)};
handle_info(
    #{status := #connected{}} = Sub,
    #mq_sub_messages{messages = Msgs}
) ->
    {ok, handle_messages(Sub, Msgs)};
handle_info(#{status := #connecting{}} = Sub0, #mq_sub_messages{
    messages = Msgs, consumer_ref = ConsumerRef
}) ->
    Sub = handle_connected(Sub0, ConsumerRef),
    {ok, handle_messages(Sub, Msgs)};
handle_info(#{status := #connecting{}} = Sub, #mq_sub_connected{consumer_ref = ConsumerRef}) ->
    {ok, handle_connected(Sub, ConsumerRef)};
%%
%% Self-initiated messages
%%
handle_info(#{status := #finding_mq{}, topic_filter := _TopicFilter} = _Sub, #find_mq_retry{}) ->
    ?tp_debug(mq_sub_find_mq_retry, #{mq_topic_filter => _TopicFilter, sub => inspect(_Sub)}),
    {error, recreate};
handle_info(
    #{status := #connecting{}, topic_filter := TopicFilter} = Sub, #consumer_connect_timeout{}
) ->
    ?tp(error, mq_sub_consumer_connect_timeout, #{
        mq_topic_filter => TopicFilter, sub => inspect(Sub)
    }),
    {error, recreate};
handle_info(
    #{status := #connected{}, topic_filter := TopicFilter} = Sub, #consumer_timeout{}
) ->
    ?tp(error, mq_sub_consumer_timeout, #{mq_topic_filter => TopicFilter, sub => inspect(Sub)}),
    {error, recreate};
handle_info(
    #{status := #connected{consumer_ref = ConsumerRef}, subscriber_ref := SubscriberRef} = Sub,
    #ping_consumer{}
) ->
    ?tp_debug(mq_sub_handle_info, #{sub => inspect(Sub), info_msg => ping}),
    ok = emqx_mq_consumer:ping(ConsumerRef, SubscriberRef),
    {ok, reset_ping_timer(Sub)};
handle_info(
    #{status := #connected{inflight = Inflight0, buffer = Buffer0, mq = MQ} = Status} = Sub0,
    #publish_retry{}
) ->
    ?tp_debug(mq_sub_handle_info_publish_retry_start, #{sub => inspect(Sub0)}),
    NPublish = max_inflight(MQ) - map_size(Inflight0),
    {MessagesWithIds, Buffer} = emqx_mq_sub_buffer:take(Buffer0, NPublish),
    {Inflight, Messages0} = lists:foldl(
        fun({MessageId, Message}, {InflightAcc, Msgs}) ->
            {InflightAcc#{MessageId => Message}, [Message | Msgs]}
        end,
        {Inflight0, []},
        MessagesWithIds
    ),
    Messages = lists:reverse(Messages0),
    Sub1 = cancel_publish_retry_timer(Sub0),
    Sub = Sub1#{status => Status#connected{inflight = Inflight, buffer = Buffer}},
    ?tp_debug(mq_sub_handle_info_publish_retry_end, #{sub => inspect(Sub), messages => Messages}),
    {ok, Sub, Messages};
handle_info(Sub, _InfoMsg) ->
    ?tp_debug(mq_sub_handle_info, #{sub => inspect(Sub), info_msg => _InfoMsg}),
    {ok, Sub}.

-spec handle_disconnect(t()) -> ok.
handle_disconnect(
    #{status := #connected{consumer_ref = ConsumerRef}, subscriber_ref := SubscriberRef} = Sub
) ->
    ok = emqx_mq_consumer:disconnect(ConsumerRef, SubscriberRef),
    destroy(Sub);
handle_disconnect(Sub) ->
    destroy(Sub).

%%--------------------------------------------------------------------
%% RPC
%%--------------------------------------------------------------------

-spec connected(emqx_mq_types:subscriber_ref(), emqx_mq_types:consumer_ref()) -> ok.
connected(SubscriberRef, ConsumerRef) when node(SubscriberRef) =:= node() ->
    connected_v1(SubscriberRef, ConsumerRef);
connected(SubscriberRef, ConsumerRef) ->
    emqx_mq_sub_proto_v1:mq_sub_connected(node(SubscriberRef), SubscriberRef, ConsumerRef).

-spec ping(emqx_mq_types:subscriber_ref()) -> ok.
ping(SubscriberRef) when node(SubscriberRef) =:= node() ->
    ping_v1(SubscriberRef);
ping(SubscriberRef) ->
    emqx_mq_sub_proto_v1:mq_sub_ping(node(SubscriberRef), SubscriberRef).

-spec messages(emqx_mq_types:subscriber_ref(), emqx_mq_types:consumer_ref(), [emqx_types:message()]) ->
    ok.
messages(SubscriberRef, ConsumerRef, Messages) when node(SubscriberRef) =:= node() ->
    messages_v1(SubscriberRef, ConsumerRef, Messages);
messages(SubscriberRef, ConsumerRef, Messages) ->
    true = emqx_mq_sub_proto_v1:mq_sub_messages(
        node(SubscriberRef), SubscriberRef, ConsumerRef, Messages
    ),
    ok.

%%--------------------------------------------------------------------
%% RPC targets
%%--------------------------------------------------------------------

-spec connected_v1(emqx_mq_types:subscriber_ref(), emqx_mq_types:consumer_ref()) -> ok.
connected_v1(SubscriberRef, ConsumerRef) ->
    send_info_to_subscriber(SubscriberRef, #mq_sub_connected{consumer_ref = ConsumerRef}).

-spec ping_v1(emqx_mq_types:subscriber_ref()) -> ok.
ping_v1(SubscriberRef) ->
    send_info_to_subscriber(SubscriberRef, #mq_sub_ping{}).

-spec messages_v1(
    emqx_mq_types:subscriber_ref(), emqx_mq_types:consumer_ref(), list(emqx_types:message())
) -> ok.
messages_v1(SubscriberRef, ConsumerRef, Messages) ->
    send_info_to_subscriber(SubscriberRef, #mq_sub_messages{
        consumer_ref = ConsumerRef, messages = Messages
    }).

%%--------------------------------------------------------------------
%% Internal functions
%%--------------------------------------------------------------------

handle_connected(#{status := #connecting{mq = MQ}} = Sub0, ConsumerRef) ->
    ?tp_debug(handle_connected, #{sub => inspect(Sub0), consumer_ref => ConsumerRef}),
    Sub = Sub0#{
        status => #connected{
            mq = MQ,
            consumer_ref = ConsumerRef,
            inflight = #{},
            buffer = emqx_mq_sub_buffer:new(),
            ping_tref = undefined,
            consumer_timeout_tref = undefined,
            publish_retry_tref = undefined
        }
    },
    reset_consumer_timeout_timer(reset_ping_timer(Sub)).

handle_messages(Sub, Msgs) ->
    lists:foldl(fun handle_message/2, Sub, Msgs).

handle_message(
    Msg,
    #{
        status := #connected{
            buffer = Buffer0, inflight = Inflight, publish_retry_tref = PublishRetryTRef, mq = MQ
        } = Status
    } = Sub0
) ->
    ?tp_debug(mq_sub_messages, #{sub => inspect(Sub0), message => Msg}),
    Buffer = emqx_mq_sub_buffer:add(Buffer0, Msg),
    Sub =
        case PublishRetryTRef of
            undefined ->
                case map_size(Inflight) < max_inflight(MQ) of
                    true ->
                        schedule_publish_retry(0, Sub0);
                    false ->
                        Sub0
                end;
            _ ->
                Sub0
        end,
    Sub#{status => Status#connected{buffer = Buffer}}.

do_handle_ack(
    #{status := #connected{inflight = Inflight0, buffer = Buffer0, mq = MQ} = Status} = Sub0,
    MessageId,
    ?MQ_NACK
) ->
    ?tp_debug(mq_sub_handle_nack, #{sub => inspect(Sub0), message_id => MessageId}),
    Message = maps:get(MessageId, Inflight0),
    Buffer = emqx_mq_sub_buffer:add(Buffer0, Message),
    Inflight = maps:remove(MessageId, Inflight0),
    Sub =
        case map_size(Inflight) of
            0 ->
                %% Channel rejected all our messages, probably it's busy.
                %% We will retry to publish the messages later.
                ?tp_debug(mq_sub_handle_nack_session_busy, #{sub => inspect(Sub0)}),
                schedule_publish_retry(retry_interval(MQ), Sub0);
            _ ->
                %% We do not try to refill the inflight buffer on NACK
                %% Threre are some messages in flight, so we
                %% wait for the next ack to refill the buffer.
                Sub0
        end,
    Sub#{
        status => Status#connected{
            inflight = Inflight,
            buffer = Buffer
        }
    };
do_handle_ack(
    #{
        status := #connected{consumer_ref = ConsumerRef, inflight = Inflight0, buffer = Buffer} =
            Status,
        subscriber_ref := SubscriberRef
    } = Sub0,
    MessageId,
    Ack
) ->
    ?tp_debug(mq_sub_handle_ack, #{sub => inspect(Sub0), message_id => MessageId, ack => Ack}),
    Inflight = maps:remove(MessageId, Inflight0),
    ok = emqx_mq_consumer:ack(ConsumerRef, SubscriberRef, MessageId, Ack),
    Sub1 =
        case emqx_mq_sub_buffer:size(Buffer) of
            0 ->
                Sub0;
            _N ->
                %% NOTE
                %% We may try to inject messages into channel just from the ack hook,
                %% without an additional message to ourselves.
                %% But we want to compete fairly with the other messages incoming to the channel.
                schedule_publish_retry(0, Sub0)
        end,
    Sub1#{
        status => Status#connected{
            inflight = Inflight
        }
    }.

destroy(#{subscriber_ref := SubscriberRef} = Sub) ->
    _ = unalias(SubscriberRef),
    _Sub = cancel_timers(Sub),
    ok.

send_info_to_subscriber(SubscriberRef, InfoMsg) ->
    _ = erlang:send(SubscriberRef, #info_to_mq_sub{
        subscriber_ref = SubscriberRef, info = InfoMsg
    }),
    ok.

%%--------------------------------------------------------------------
%% Timers
%%--------------------------------------------------------------------

reset_consumer_timeout_timer(
    #{status := #connected{consumer_timeout_tref = TRef, mq = MQ} = Status} = Sub
) ->
    ?tp_debug(mq_sub_reset_consumer_timeout_timer, #{sub => inspect(Sub)}),
    _ = emqx_utils:cancel_timer(TRef),
    Sub#{
        status => Status#connected{
            consumer_timeout_tref = send_after(
                Sub, consumer_timeout(MQ), #consumer_timeout{}
            )
        }
    }.

reset_ping_timer(#{status := #connected{ping_tref = TRef, mq = MQ} = Status} = Sub) ->
    _ = emqx_utils:cancel_timer(TRef),
    Sub#{
        status => Status#connected{
            ping_tref = send_after(Sub, ping_interval(MQ), #ping_consumer{})
        }
    }.

cancel_publish_retry_timer(#{status := #connected{publish_retry_tref = TRef} = Status} = Sub) ->
    _ = emqx_utils:cancel_timer(TRef),
    Sub#{
        status => Status#connected{
            publish_retry_tref = undefined
        }
    }.

cancel_timers(#{status := #finding_mq{find_mq_retry_tref = TRef} = Status} = Sub) ->
    _ = emqx_utils:cancel_timer(TRef),
    Sub#{
        status => Status#finding_mq{
            find_mq_retry_tref = undefined
        }
    };
cancel_timers(#{status := #connecting{connect_timeout_tref = TRef} = Status} = Sub) ->
    _ = emqx_utils:cancel_timer(TRef),
    Sub#{
        status => Status#connecting{
            connect_timeout_tref = undefined
        }
    };
cancel_timers(
    #{
        status := #connected{
            consumer_timeout_tref = TRef,
            ping_tref = PingTRef,
            publish_retry_tref = PublishRetryTRef
        } = Status
    } = Sub
) ->
    _ = emqx_utils:cancel_timer(TRef),
    _ = emqx_utils:cancel_timer(PingTRef),
    _ = emqx_utils:cancel_timer(PublishRetryTRef),
    Sub#{
        status => Status#connected{
            consumer_timeout_tref = undefined,
            ping_tref = undefined,
            publish_retry_tref = undefined
        }
    }.

schedule_publish_retry(Interval, #{status := #connected{} = Status} = Sub0) ->
    Sub = cancel_publish_retry_timer(Sub0),
    ?tp_debug(mq_sub_schedule_publish_retry, #{sub => inspect(Sub), interval => Interval}),
    Sub#{
        status => Status#connected{
            publish_retry_tref = send_after(Sub, Interval, #publish_retry{})
        }
    }.

send_after(#{subscriber_ref := SubscriberRef}, Interval, InfoMessage) ->
    erlang:send_after(
        Interval, self(), #info_to_mq_sub{
            subscriber_ref = SubscriberRef, info = InfoMessage
        }
    ).

%%--------------------------------------------------------------------
%% MQ settings
%%--------------------------------------------------------------------

%% NOTE
%% Cannot be configured individually for each MQ
%% because MQ is still not known when we try to find it.
find_mq_retry_interval() ->
    emqx_config:get([mq, find_queue_retry_interval]).

ping_interval(#{ping_interval := ConsumerPingIntervalMs} = _MQ) ->
    ConsumerPingIntervalMs.

retry_interval(#{busy_session_retry_interval := BusySessionRetryInterval} = _MQ) ->
    BusySessionRetryInterval.

consumer_timeout(MQ) ->
    ping_interval(MQ) * 2.

max_inflight(#{local_max_inflight := LocalMaxInflight} = _MQ) ->
    LocalMaxInflight.

connect_retry_interval(no_mq) ->
    find_mq_retry_interval();
connect_retry_interval(already_registered) ->
    ?CONNECT_RETRY_INTERVAL_ALREADY_REGISTERED;
connect_retry_interval(_Reason) ->
    ?CONNECT_RETRY_INTERVAL_UNEXPECTED_ERROR.

%%--------------------------------------------------------------------
%% Introspection helpers
%%--------------------------------------------------------------------

status_inspect(
    #connected{
        buffer = Buffer,
        inflight = Inflight,
        consumer_ref = ConsumerRef,
        consumer_timeout_tref = ConsumerTimeoutTRef,
        ping_tref = PingTRef,
        publish_retry_tref = PublishRetryTRef
    }
) ->
    #{
        name => connected,
        buffer_size => emqx_mq_sub_buffer:size(Buffer),
        inflight => maps:keys(Inflight),
        consumer_ref => ConsumerRef,
        consumer_timeout_tref => ConsumerTimeoutTRef,
        ping_tref => PingTRef,
        publish_retry_tref => PublishRetryTRef
    };
status_inspect(#connecting{}) ->
    #{name => connecting};
status_inspect(#finding_mq{}) ->
    #{name => finding_mq}.
