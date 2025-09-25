%%--------------------------------------------------------------------
%% Copyright (c) 2025 EMQ Technologies Co., Ltd. All Rights Reserved.
%%--------------------------------------------------------------------

-module(emqx_mq_consumer_server).

-moduledoc """
The module is responsible for interacting with the MQ consumer subscribers, i.e.
channels subscribed to a Message Queue.
""".

-include("../emqx_mq_internal.hrl").
-include_lib("emqx/include/emqx_mqtt.hrl").
-include_lib("snabbkaffe/include/snabbkaffe.hrl").

-export([
    new/1,
    handle_messages/2,
    handle_info/2
]).

-export([
    inspect/1
]).

%%--------------------------------------------------------------------
%% Types
%%--------------------------------------------------------------------

-type subscriber_ref() :: emqx_mq_types:subscriber_ref().
-type message_id() :: emqx_mq_types:message_id().
-type monotonic_timestamp_ms() :: emqx_mq_types:monotonic_timestamp_ms().
-type subscriber_data() :: #{
    timeout_tref := reference(),
    client_id := emqx_types:clientid(),
    inflight_messages := #{message_id() => monotonic_timestamp_ms()},
    last_ack_ts := monotonic_timestamp_ms()
}.

-define(ping_timer, ping_timer).
-define(shutdown_timer, shutdown_timer).
-define(dispatch_timer, dispatch_timer).

-type round_robin_iterator() :: maps:iterator(subscriber_ref(), subscriber_data()).

-type dispatch_strategy() :: random | least_inflight | {round_robin, round_robin_iterator()}.

-record(state, {
    mq :: emqx_mq_types:mq(),
    subscribers :: #{subscriber_ref() => subscriber_data()},
    messages :: #{message_id() => emqx_types:message()},
    dispatch_queue :: emqx_mq_consumer_dispatchq:t(),
    dispatch_strategy :: dispatch_strategy(),
    timers :: #{
        ?ping_timer => emqx_maybe:t(reference()),
        ?shutdown_timer => emqx_maybe:t(reference()),
        ?dispatch_timer => emqx_maybe:t(reference())
    }
}).

-type t() :: #state{}.

-type event() :: {ds_ack, message_id()} | shutdown.

-export_type([t/0, event/0]).

%%--------------------------------------------------------------------
%% Messages
%%--------------------------------------------------------------------

-record(timer_message, {timer_name :: ?ping_timer | ?shutdown_timer | ?dispatch_timer}).

-record(subscriber_timeout, {
    subscriber_ref :: subscriber_ref()
}).

%%--------------------------------------------------------------------
%% API
%%--------------------------------------------------------------------

-spec new(emqx_mq_types:mq()) -> t().
new(MQ) ->
    #state{
        mq = MQ,
        subscribers = #{},
        messages = #{},
        dispatch_strategy = dispatch_strategy(MQ),
        dispatch_queue = emqx_mq_consumer_dispatchq:new(),
        timers = #{
            ?ping_timer => undefined,
            ?shutdown_timer => undefined,
            ?dispatch_timer => undefined
        }
    }.

-spec handle_messages(t(), [{message_id(), emqx_types:message()}]) -> t().
handle_messages(State, Messages) ->
    dispatch(add_new_messages(State, Messages)).

-spec handle_info(t(), term()) -> {ok, [event()], t()}.
handle_info(State, #mq_server_connect{subscriber_ref = SubscriberRef, client_id = ClientId}) ->
    {ok, [], handle_connect(State, SubscriberRef, ClientId)};
handle_info(State, #mq_server_disconnect{subscriber_ref = SubscriberRef}) ->
    {ok, [], handle_disconnect(State, SubscriberRef)};
handle_info(State, #mq_server_ack{subscriber_ref = SubscriberRef, message_id = MessageId, ack = Ack}) ->
    handle_ack(State, SubscriberRef, MessageId, Ack);
handle_info(State, #mq_server_ping{subscriber_ref = SubscriberRef}) ->
    {ok, [], handle_ping(State, SubscriberRef)};
handle_info(State, #timer_message{timer_name = ?ping_timer}) ->
    {ok, [], handle_ping_subscribers(State)};
handle_info(State, #timer_message{timer_name = ?shutdown_timer}) ->
    handle_shutdown(State);
handle_info(State, #timer_message{timer_name = ?dispatch_timer}) ->
    {ok, [], handle_dispatch(State)};
handle_info(State, #subscriber_timeout{subscriber_ref = SubscriberRef}) ->
    {ok, [], handle_subscriber_timeout(State, SubscriberRef)}.

inspect(#state{
    subscribers = Subscribers, messages = Messages, dispatch_queue = DispatchQueue, timers = Timers
}) ->
    #{
        subscribers => lists:map(fun subscriber_inspect/1, maps:to_list(Subscribers)),
        messages => map_size(Messages),
        dispatch_queue => emqx_mq_consumer_dispatchq:size(DispatchQueue),
        timers => [Name || {Name, Timer} <- maps:to_list(Timers), Timer =/= undefined]
    }.

%%--------------------------------------------------------------------
%% Internal functions
%%--------------------------------------------------------------------

subscriber_inspect(
    {_SubscriberRef, #{
        client_id := ClientId, inflight_messages := InflightMessages, last_ack_ts := LastAckTs
    }}
) ->
    #{
        client_id => ClientId,
        inflight_messages => map_size(InflightMessages),
        last_ack_ts_ago_ms => now_ms_monotonic() - LastAckTs
    }.

handle_connect(#state{subscribers = Subscribers0} = State, SubscriberRef, ClientId) ->
    ?tp_debug(mq_consumer_handle_connect, #{
        subscriber_ref => SubscriberRef, client_id => ClientId
    }),
    case Subscribers0 of
        #{SubscriberRef := SubscriberData0} ->
            SubscriberData1 = refresh_subscriber_timeout(State, SubscriberRef, SubscriberData0),
            Subscribers1 = Subscribers0#{SubscriberRef => SubscriberData1},
            State#state{subscribers = Subscribers1};
        _ ->
            Subscribers1 = Subscribers0#{
                SubscriberRef => initial_subscriber_data(State, SubscriberRef, ClientId)
            },
            ok = send_connected_to_subscriber(SubscriberRef),
            dispatch(handle_subscriber_change(State#state{subscribers = Subscribers1}))
    end.

handle_disconnect(#state{subscribers = Subscribers0} = State0, SubscriberRef) ->
    case Subscribers0 of
        #{SubscriberRef := #{inflight_messages := InflightMessages} = SubscriberData} ->
            _ = cancel_subscriber_timeout(SubscriberData),
            Subscribers = maps:remove(SubscriberRef, Subscribers0),
            State1 = State0#state{subscribers = Subscribers},
            State = schedule_for_dispatch(State1, maps:keys(InflightMessages)),
            dispatch(handle_subscriber_change(State));
        _ ->
            State0
    end.

handle_ack(
    #state{subscribers = Subscribers0, messages = Messages0} = State0,
    SubscriberRef,
    MessageId,
    Ack
) ->
    ?tp_debug(mq_consumer_handle_ack, #{
        subscriber_ref => SubscriberRef, message_id => MessageId, ack => Ack
    }),
    #{SubscriberRef := SubscriberData0} = Subscribers0,
    #{inflight_messages := InflightMessages0} = SubscriberData0,
    #{MessageId := _} = InflightMessages0,
    InflightMessages = maps:remove(MessageId, InflightMessages0),
    SubscriberData1 = SubscriberData0#{inflight_messages := InflightMessages},
    SubscriberData2 = refresh_subscriber_timeout(State0, SubscriberRef, SubscriberData1),
    SubscriberData3 = update_last_ack(SubscriberData2),
    Subscribers = Subscribers0#{SubscriberRef => SubscriberData3},
    State1 = State0#state{subscribers = Subscribers},
    case Ack of
        ?MQ_ACK ->
            Messages = maps:remove(MessageId, Messages0),
            State = State1#state{messages = Messages},
            {ok, [{ds_ack, MessageId}], State};
        ?MQ_REJECTED ->
            {ok, [], redispatch_on_reject(State1, SubscriberRef, MessageId)}
    end.

handle_ping(#state{subscribers = Subscribers0} = State, SubscriberRef) ->
    ?tp_debug(mq_consumer_handle_ping, #{subscriber_ref => SubscriberRef}),
    case Subscribers0 of
        #{SubscriberRef := SubscriberData0} ->
            SubscriberData = refresh_subscriber_timeout(State, SubscriberRef, SubscriberData0),
            Subscribers = Subscribers0#{SubscriberRef => SubscriberData},
            State#state{subscribers = Subscribers};
        _ ->
            State
    end.

handle_ping_subscribers(#state{subscribers = Subscribers} = State0) ->
    ?tp_debug(mq_consumer_handle_ping_subscribers, #{subscribers => maps:keys(Subscribers)}),
    State1 = cancel_timer(?ping_timer, State0),
    ok = maps:foreach(
        fun(SubscriberRef, _SubscriberData) ->
            send_ping_to_subscriber(SubscriberRef)
        end,
        Subscribers
    ),
    ensure_timer(?ping_timer, State1).

handle_subscriber_timeout(State, SubscriberRef) ->
    handle_disconnect(State, SubscriberRef).

handle_dispatch(State) ->
    dispatch(State).

handle_shutdown(State) ->
    {ok, [shutdown], State}.

%%--------------------------------------------------------------------
%% Internal functions
%%--------------------------------------------------------------------

add_new_messages(#state{messages = Messages0} = State, MessagesWithIds) ->
    {MessageIds, Messages} = lists:mapfoldl(
        fun({MessageId, Message}, Acc) ->
            {MessageId, Acc#{MessageId => Message}}
        end,
        Messages0,
        MessagesWithIds
    ),
    schedule_for_dispatch(State#state{messages = Messages}, MessageIds).

refresh_subscriber_timeout(State, SubscriberRef, SubscriberData0) ->
    SubscriberData = cancel_subscriber_timeout(SubscriberData0),
    TimeoutTRef = send_after(
        2 * timeout(?ping_timer, State),
        #subscriber_timeout{subscriber_ref = SubscriberRef}
    ),
    SubscriberData#{timeout_tref := TimeoutTRef}.

update_last_ack(SubscriberData0) ->
    SubscriberData0#{last_ack_ts := now_ms_monotonic()}.

cancel_subscriber_timeout(#{timeout_tref := TimeoutTRef} = SubscriberData) ->
    emqx_utils:cancel_timer(TimeoutTRef),
    SubscriberData#{timeout_tref := undefined}.

initial_subscriber_data(State, SubscriberRef, ClientId) ->
    SubscriberData = #{
        timeout_tref => undefined,
        client_id => ClientId,
        inflight_messages => #{},
        last_ack_ts => now_ms_monotonic()
    },
    refresh_subscriber_timeout(State, SubscriberRef, SubscriberData).

schedule_for_dispatch(#state{dispatch_queue = DispatchQueue0} = State, MessageIds) ->
    DispatchQueue1 = emqx_mq_consumer_dispatchq:add(DispatchQueue0, MessageIds),
    State#state{dispatch_queue = DispatchQueue1}.

redispatch_on_reject(
    #state{dispatch_queue = DispatchQueue0} = State0, RejectedSubscriberRef, MessageId
) ->
    ?tp_debug(mq_consumer_redispatch_on_reject, #{
        rejected_subscriber_ref => RejectedSubscriberRef, message_id => MessageId
    }),
    case dispatch_action(MessageId, [RejectedSubscriberRef], State0) of
        {dispatch, SubscriberRef, State} ->
            dispatch_to_subscriber([MessageId], SubscriberRef, State);
        {delay, Interval, State} ->
            %% NOTE
            %% to enable timer
            DispatchQueue = emqx_mq_consumer_dispatchq:add_redispatch(
                DispatchQueue0, MessageId, Interval
            ),
            dispatch(State#state{dispatch_queue = DispatchQueue})
    end.

dispatch(#state{subscribers = Subscribers} = State) when map_size(Subscribers) =:= 0 ->
    cancel_timer(?dispatch_timer, State);
dispatch(State0) ->
    {State1, MessagesBySubscriber} = collect_messages_for_dispatch(State0, #{}),
    maps:fold(
        fun(SubscriberRef, MessageIds, StateAcc) ->
            dispatch_to_subscriber(lists:reverse(MessageIds), SubscriberRef, StateAcc)
        end,
        State1,
        MessagesBySubscriber
    ).

collect_messages_for_dispatch(
    #state{dispatch_queue = DispatchQueue0} = State0, MessagesBySubscriber0
) ->
    State1 = cancel_timer(?dispatch_timer, State0),
    case emqx_mq_consumer_dispatchq:fetch(DispatchQueue0) of
        empty ->
            {State1, MessagesBySubscriber0};
        {ok, MessageIds, DispatchQueue1} ->
            State2 = State1#state{dispatch_queue = DispatchQueue1},
            {MessagesBySubscriber, State} = lists:foldl(
                fun(MessageId, {MessagesBySubscriberAcc, StateAcc0}) ->
                    case dispatch_action(MessageId, [], StateAcc0) of
                        %% Acumulate the message for sending to the selected subscriber
                        {dispatch, SubscriberRef, StateAcc} ->
                            {
                                maps:update_with(
                                    SubscriberRef,
                                    fun(SubscriberMessageIds) ->
                                        [MessageId | SubscriberMessageIds]
                                    end,
                                    [MessageId],
                                    MessagesBySubscriberAcc
                                ),
                                StateAcc
                            };
                        %% Do not acumulate the message, put it back to the dispatch queue
                        %% for further redispatch
                        {delay, Interval, #state{dispatch_queue = DispatchQueueAcc} = StateAcc} ->
                            {MessagesBySubscriberAcc, StateAcc#state{
                                dispatch_queue =
                                    emqx_mq_consumer_dispatchq:add_redispatch(
                                        DispatchQueueAcc, MessageId, Interval
                                    )
                            }}
                    end
                end,
                {MessagesBySubscriber0, State2},
                MessageIds
            ),
            collect_messages_for_dispatch(State, MessagesBySubscriber);
        {delay, DelayMs} ->
            State = ensure_timer(?dispatch_timer, DelayMs, State1),
            {State, MessagesBySubscriber0}
    end.

-spec dispatch_action(message_id(), [subscriber_ref()], t()) ->
    {dispatch, subscriber_ref(), t()} | {delay, pos_integer(), t()}.
dispatch_action(
    MessageId,
    ExcludedSubscriberRefs,
    #state{messages = Messages, subscribers = _Subscribers} = State0
) ->
    Message = maps:get(MessageId, Messages),
    {PickResult, State} = pick_subscriber(Message, ExcludedSubscriberRefs, State0),
    ?tp_debug(mq_consumer_dispatch_message, #{
        message_id => MessageId,
        pick_result => PickResult,
        subscribers => maps:keys(_Subscribers),
        excluded_subscriber_refs => ExcludedSubscriberRefs,
        message_topic => emqx_message:topic(Message)
    }),
    case PickResult of
        {ok, SubscriberRef} ->
            {dispatch, SubscriberRef, State};
        no_subscriber ->
            {delay, redispatch_interval(State), State}
    end.

dispatch_to_subscriber(
    MessageIds, SubscriberRef, #state{subscribers = Subscribers0, messages = Messages} = State0
) ->
    #{inflight_messages := InflightMessages0} =
        SubscriberData0 = maps:get(SubscriberRef, Subscribers0),
    InflightMessages = add_message_ids_to_inflight(MessageIds, InflightMessages0),
    SubscriberData = SubscriberData0#{inflight_messages := InflightMessages},
    Subscribers = Subscribers0#{SubscriberRef => SubscriberData},
    State = State0#state{subscribers = Subscribers},
    MessagesToSend = enrich_messages_with_id(MessageIds, Messages),
    ok = send_messages_to_subscriber(SubscriberRef, MessagesToSend),
    State.

handle_subscriber_change(State0) ->
    State = update_dispatch_strategy(State0),
    update_client_timers(State).

update_client_timers(#state{subscribers = Subscribers} = State0) when map_size(Subscribers) =:= 0 ->
    State1 = cancel_timer(?ping_timer, State0),
    State2 = cancel_timer(?dispatch_timer, State1),
    ensure_timer(?shutdown_timer, State2);
update_client_timers(#state{} = State0) ->
    State1 = ensure_timer(?ping_timer, State0),
    cancel_timer(?shutdown_timer, State1).

update_dispatch_strategy(
    #state{dispatch_strategy = {round_robin, _}, subscribers = Subscribers} = State
) ->
    State#state{dispatch_strategy = {round_robin, maps:iterator(Subscribers)}};
update_dispatch_strategy(State) ->
    State.

%%--------------------------------------------------------------------
%% Dispatch strategies
%%--------------------------------------------------------------------

%% NOTE
%% Currently, no strategy actually takes the message into account.
pick_subscriber(_Message, ExcludedSubscriberRefs, #state{dispatch_strategy = random} = State) ->
    {pick_subscriber_random(ExcludedSubscriberRefs, State), State};
pick_subscriber(
    _Message, ExcludedSubscriberRefs, #state{dispatch_strategy = least_inflight} = State
) ->
    {pick_subscriber_least_inflight(ExcludedSubscriberRefs, State), State};
pick_subscriber(
    _Message, ExcludedSubscriberRefs, #state{dispatch_strategy = {round_robin, _}} = State
) ->
    pick_subscriber_round_robin(ExcludedSubscriberRefs, State).

%% Random dispatch strategy

pick_subscriber_random(
    ExcludedSubscriberRefs, #state{subscribers = Subscribers} = _State
) ->
    case maps:keys(Subscribers) -- ExcludedSubscriberRefs of
        [] ->
            no_subscriber;
        SubscriberRefs ->
            RandIndex = rand:uniform(length(SubscriberRefs)),
            {ok, lists:nth(RandIndex, SubscriberRefs)}
    end.

%% Least inflight dispatch strategy

pick_subscriber_least_inflight(
    ExcludedSubscriberRefs, #state{subscribers = Subscribers} = _State
) ->
    {SubscriberRef, _} = maps:fold(
        fun(
            SubscriberRef,
            #{inflight_messages := InflightMessages} = _SubscriberData,
            {LeastSubscriberRef, LeastInflightCount}
        ) ->
            case map_size(InflightMessages) of
                InflightCount when InflightCount < LeastInflightCount ->
                    {SubscriberRef, InflightCount};
                _ ->
                    {LeastSubscriberRef, LeastInflightCount}
            end
        end,
        {undefined, infinity},
        maps:without(ExcludedSubscriberRefs, Subscribers)
    ),
    case SubscriberRef of
        undefined ->
            no_subscriber;
        _ ->
            {ok, SubscriberRef}
    end.

%% Round robin dispatch strategy

pick_subscriber_round_robin(
    ExcludedSubscriberRefs,
    #state{dispatch_strategy = {round_robin, Iterator0}, subscribers = Subscribers} = State
) ->
    Seen0 = maps:from_keys(ExcludedSubscriberRefs, 0),
    Seen = Seen0#{none => 0},
    InitialNextValue = maps:next(Iterator0),
    ?tp_debug(mq_consumer_pick_subscriber_round_robin, #{
        iterator => Iterator0,
        seen => Seen,
        initial_next_iterator => InitialNextValue
    }),
    {Iterator, Result} = find_next_subscriber(InitialNextValue, Subscribers, Seen),
    {Result, State#state{dispatch_strategy = {round_robin, Iterator}}}.

find_next_subscriber(none, Subscribers, #{none := 0} = Seen) ->
    find_next_subscriber(maps:next(maps:iterator(Subscribers)), Subscribers, Seen#{none => 1});
find_next_subscriber(none, Subscribers, #{none := 1} = _Seen) ->
    {maps:iterator(Subscribers), no_subscriber};
find_next_subscriber({SubscriberRef, _SubscriberData, NextIterator}, Subscribers, Seen) ->
    case Seen of
        #{SubscriberRef := 0} ->
            find_next_subscriber(maps:next(NextIterator), Subscribers, Seen#{SubscriberRef => 1});
        #{SubscriberRef := 1} ->
            {NextIterator, no_subscriber};
        _ ->
            {NextIterator, {ok, SubscriberRef}}
    end.

%%--------------------------------------------------------------------
%% Helpers
%%--------------------------------------------------------------------

ensure_timer(TimerName, State) ->
    ensure_timer(TimerName, timeout(TimerName, State), State).

ensure_timer(TimerName, Timeout, #state{timers = Timers0} = State) ->
    Timers =
        case Timers0 of
            #{TimerName := undefined} ->
                TRref = send_after(Timeout, #timer_message{timer_name = TimerName}),
                Timers0#{TimerName => TRref};
            _ ->
                Timers0
        end,
    State#state{timers = Timers}.

cancel_timer(TimerName, #state{timers = Timers0} = State) ->
    Timers =
        case Timers0 of
            #{TimerName := undefined} ->
                Timers0;
            #{TimerName := TimerRef} ->
                _ = emqx_utils:cancel_timer(TimerRef),
                Timers0#{TimerName => undefined}
        end,
    State#state{timers = Timers}.

send_after(Timeout, Message) ->
    erlang:send_after(Timeout, self_consumer_ref(), #info_to_mq_server{message = Message}).

now_ms_monotonic() ->
    erlang:monotonic_time(millisecond).

timeout(?ping_timer, #state{mq = #{ping_interval := ConsumerPingIntervalMs}}) ->
    ConsumerPingIntervalMs;
timeout(?shutdown_timer, #state{mq = #{consumer_max_inactive := ConsumerMaxInactiveMs}}) ->
    ConsumerMaxInactiveMs.

dispatch_strategy(#{dispatch_strategy := random}) ->
    random;
dispatch_strategy(#{dispatch_strategy := least_inflight}) ->
    least_inflight;
dispatch_strategy(#{dispatch_strategy := round_robin}) ->
    % will be initialized when the first subscriber connects
    {round_robin, maps:iterator(#{})};
dispatch_strategy(_) ->
    random.

redispatch_interval(#state{mq = #{redispatch_interval := RedispatchIntervalMs}}) ->
    RedispatchIntervalMs.

add_message_ids_to_inflight(MessageIds, InflightMessages0) ->
    NowMsMonotonic = now_ms_monotonic(),
    lists:foldl(
        fun(MessageId, InflightMessagesAcc) ->
            InflightMessagesAcc#{MessageId => NowMsMonotonic}
        end,
        InflightMessages0,
        MessageIds
    ).

send_connected_to_subscriber(SubscriberRef) ->
    ok = emqx_mq_sub:connected(SubscriberRef, self_consumer_ref()).

send_ping_to_subscriber(SubscriberRef) ->
    ok = emqx_mq_sub:ping(SubscriberRef).

enrich_messages_with_id(MessageIds, Messages) ->
    lists:map(
        fun(MessageId) ->
            emqx_message:set_headers(
                #{?MQ_HEADER_MESSAGE_ID => MessageId}, maps:get(MessageId, Messages)
            )
        end,
        MessageIds
    ).

send_messages_to_subscriber(SubscriberRef, Messages) ->
    emqx_mq_sub:messages(SubscriberRef, self_consumer_ref(), Messages).

self_consumer_ref() ->
    self().
