%%--------------------------------------------------------------------
%% Copyright (c) 2025 EMQ Technologies Co., Ltd. All Rights Reserved.
%%--------------------------------------------------------------------

-module(emqx_mq_perf).

-moduledoc """
Performance test utilities for the MQ application.
""".

-ifdef(MQ_PERF).

-include_lib("../emqx_mq_internal.hrl").

-export([
    create_mq_regular/1,
    mq_regular/1,
    populate_regular/3,
    cleanup_mq_regular_consumption_progress/1,
    subscriber_inspect/1,
    consumer_inspect_regular/1,
    inspect_regular/1
]).

-define(MQ_REGULAR, #{
    is_lastvalue => false,
    consumer_max_inactive => 1000,
    ping_interval => 10000,
    redispatch_interval => 100,
    dispatch_strategy => random,
    local_max_inflight => 20,
    busy_session_retry_interval => 100,
    stream_max_buffer_size => 100,
    stream_max_unacked => 100,
    consumer_persistence_interval => 10000,
    data_retention_period => 3600_000
}).

%%--------------------------------------------------------------------
%% API
%%--------------------------------------------------------------------

create_mq_regular(TopicFilter) ->
    _ = emqx_mq_registry:delete(TopicFilter),
    {ok, MQ} = emqx_mq_registry:create(maps:merge(?MQ_REGULAR, #{topic_filter => TopicFilter})),
    ok = wait_for_mq_created(MQ),
    MQ.

mq_regular(TopicFilter) ->
    emqx_mq_registry:find(TopicFilter).

cleanup_mq_regular_consumption_progress(TopicFilter) when is_binary(TopicFilter) ->
    ok = stop_all_consumers(),
    {ok, MQ} = emqx_mq_registry:find(TopicFilter),
    ok = emqx_mq_state_storage:destroy_consumer_state(to_handle(MQ));
cleanup_mq_regular_consumption_progress(TopicFilters) when is_list(TopicFilters) ->
    lists:foreach(
        fun(TopicFilter) ->
            cleanup_mq_regular_consumption_progress(TopicFilter)
        end,
        TopicFilters
    ).

populate_regular(TopicFilter, N, BatchSize) ->
    MQ = create_mq_regular(TopicFilter),
    populate_regular(MQ, N, 0, now_ms_monotonic(), BatchSize).

inspect_regular(TopicFilter) ->
    #{
        subscribers => subscriber_inspect(TopicFilter),
        consumer => consumer_inspect_regular(TopicFilter)
    }.

subscriber_inspect(TopicFilter) ->
    lists:flatmap(
        fun(ChanPid) ->
            case emqx_mq:inspect(ChanPid, TopicFilter) of
                undefined ->
                    [];
                Info ->
                    [Info]
            end
        end,
        emqx_cm:all_channels()
    ).

consumer_inspect_regular(TopicFilter) ->
    {ok, #{id := Id} = _MQ} = emqx_mq_registry:find(TopicFilter),
    case emqx_mq_consumer:find(Id) of
        {ok, Pid} ->
            try
                emqx_mq_consumer:inspect(Pid, 1000)
            catch
                exit:{timeout, _} ->
                    #{error => timeout, pid => Pid}
            end;
        Other ->
            Other
    end.

%%--------------------------------------------------------------------
%% Internal functions
%%--------------------------------------------------------------------

populate_regular(#{topic_filter := TopicFilter} = MQ, N, NGenerated, StartTime, BatchSize) when
    NGenerated < N
->
    ActualBatchSize = min(BatchSize, N - NGenerated),
    Messages = generate_regular_messages(NGenerated, NGenerated + ActualBatchSize - 1),
    ok = emqx_mq_message_db:insert(MQ, Messages),
    io:format("populate_regular tf=~p generated ~p(~p), elapsed ~p~n", [
        TopicFilter, N, NGenerated, now_ms_monotonic() - StartTime
    ]),
    populate_regular(MQ, N, NGenerated + BatchSize, StartTime, BatchSize);
populate_regular(_MQ, _N, _NGenerated, _StartTime, _BatchSize) ->
    ok.

generate_regular_messages(FromN, ToN) ->
    lists:map(
        fun(I) ->
            IBin = integer_to_binary(I),
            Payload = <<"payload-", IBin/binary>>,
            Topic = <<"test/", IBin/binary>>,
            ClientId = <<"client-", IBin/binary>>,
            emqx_message:make(ClientId, Topic, Payload)
        end,
        lists:seq(FromN, ToN)
    ).

wait_for_mq_created(#{topic_filter := TopicFilter} = MQ) ->
    case emqx_mq_registry:find(TopicFilter) of
        {ok, MQ} ->
            ok;
        not_found ->
            timer:sleep(100),
            wait_for_mq_created(MQ)
    end.

now_ms_monotonic() ->
    erlang:monotonic_time(millisecond).

stop_all_consumers() ->
    ConsumerPids = [
        Pid
     || {_, Pid, _, _} <- supervisor:which_children(emqx_mq_consumer_sup), is_pid(Pid)
    ],
    ok = lists:foreach(
        fun(Pid) ->
            ok = emqx_mq_consumer:stop(Pid)
        end,
        ConsumerPids
    ).

to_handle(#{id := Id, topic_filter := TopicFilter, is_lastvalue := IsLastvalue} = _MQ) ->
    #{id => Id, topic_filter => TopicFilter, is_lastvalue => IsLastvalue}.

%% MQ_PERF
-endif.
